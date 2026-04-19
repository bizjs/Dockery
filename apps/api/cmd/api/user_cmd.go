package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"api/internal/biz"
	"api/internal/conf"
	"api/internal/data"

	"github.com/go-kratos/kratos/v2/log"
)

// runUserCommand is the entry point for `dockery-api user <verb>`.
// It wires just enough of the biz layer to operate on the users table
// without starting the HTTP server.
//
// Usage:
//
//	dockery-api [-conf PATH] user list
//	dockery-api [-conf PATH] user create <username> <role>    # prompts for password
//	dockery-api [-conf PATH] user passwd <username>           # prompts for new password
//	dockery-api [-conf PATH] user grant  <username> <pattern>[,<pattern>...]
//	dockery-api [-conf PATH] user revoke <permission-id>
//	dockery-api [-conf PATH] user delete <username>
func runUserCommand(args []string, dataConf *conf.Data, logger log.Logger) int {
	if len(args) == 0 {
		printUserUsage()
		return 2
	}

	d, cleanup, err := data.NewData(dataConf, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "data: %v\n", err)
		return 1
	}
	defer cleanup()

	userRepo := data.NewUserRepo(d, logger)
	permRepo := data.NewPermissionRepo(d, logger)
	users := biz.NewUserUsecase(userRepo)
	perms := biz.NewPermissionUsecase(permRepo, userRepo)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	switch args[0] {
	case "list":
		return userList(ctx, users)
	case "create":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: user create <username> <admin|write|view>")
			return 2
		}
		return userCreate(ctx, users, args[1], args[2])
	case "passwd":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: user passwd <username>")
			return 2
		}
		return userPasswd(ctx, users, args[1])
	case "grant":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: user grant <username> <pattern>[,<pattern>...]")
			return 2
		}
		return userGrant(ctx, users, perms, args[1], args[2])
	case "revoke":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: user revoke <permission-id>")
			return 2
		}
		return userRevoke(ctx, perms, args[1])
	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: user delete <username>")
			return 2
		}
		return userDelete(ctx, users, args[1])
	default:
		printUserUsage()
		return 2
	}
}

func printUserUsage() {
	fmt.Fprintln(os.Stderr, `Usage: dockery-api [-conf PATH] user <verb> [args...]

Verbs:
  list                              list every user
  create <username> <role>          create a user (role: admin|write|view); prompts for password
  passwd <username>                 reset password; prompts for new password
  grant  <username> <patterns>      grant comma-separated repo patterns
  revoke <permission-id>            delete one permission row by id
  delete <username>                 delete a user and their permissions`)
}

func userList(ctx context.Context, u *biz.UserUsecase) int {
	users, err := u.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list: %v\n", err)
		return 1
	}
	fmt.Printf("%-5s %-20s %-8s %-8s\n", "ID", "USERNAME", "ROLE", "STATUS")
	for _, x := range users {
		status := "active"
		if x.Disabled {
			status = "disabled"
		}
		fmt.Printf("%-5d %-20s %-8s %-8s\n", x.ID, x.Username, x.Role, status)
	}
	return 0
}

func userCreate(ctx context.Context, u *biz.UserUsecase, username, role string) int {
	pass, err := promptPassword("Password: ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read password: %v\n", err)
		return 1
	}
	user, err := u.Create(ctx, username, pass, role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create: %v\n", err)
		return 1
	}
	fmt.Printf("created: id=%d username=%s role=%s\n", user.ID, user.Username, user.Role)
	return 0
}

func userPasswd(ctx context.Context, u *biz.UserUsecase, username string) int {
	user, err := u.GetByUsername(ctx, username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup: %v\n", err)
		return 1
	}
	pass, err := promptPassword("New password: ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read password: %v\n", err)
		return 1
	}
	if err := u.SetPassword(ctx, user.ID, pass); err != nil {
		fmt.Fprintf(os.Stderr, "passwd: %v\n", err)
		return 1
	}
	fmt.Printf("password updated for %s\n", username)
	return 0
}

func userGrant(ctx context.Context, u *biz.UserUsecase, p *biz.PermissionUsecase, username, patternsCSV string) int {
	user, err := u.GetByUsername(ctx, username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup: %v\n", err)
		return 1
	}
	patterns := strings.Split(patternsCSV, ",")
	rows, err := p.GrantPatterns(ctx, user.ID, patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grant: %v\n", err)
		return 1
	}
	fmt.Printf("granted %d pattern(s) to %s\n", len(rows), username)
	for _, r := range rows {
		fmt.Printf("  id=%d pattern=%s\n", r.ID, r.RepoPattern)
	}
	return 0
}

func userRevoke(ctx context.Context, p *biz.PermissionUsecase, idStr string) int {
	id, err := atoi(idStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid id %q\n", idStr)
		return 2
	}
	if err := p.Revoke(ctx, id); err != nil {
		if errors.Is(err, biz.ErrPermissionNotFound) {
			fmt.Fprintf(os.Stderr, "no such permission id=%d\n", id)
			return 1
		}
		fmt.Fprintf(os.Stderr, "revoke: %v\n", err)
		return 1
	}
	fmt.Printf("revoked permission id=%d\n", id)
	return 0
}

func userDelete(ctx context.Context, u *biz.UserUsecase, username string) int {
	user, err := u.GetByUsername(ctx, username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup: %v\n", err)
		return 1
	}
	if err := u.Delete(ctx, user.ID); err != nil {
		fmt.Fprintf(os.Stderr, "delete: %v\n", err)
		return 1
	}
	fmt.Printf("deleted user %s (id=%d)\n", username, user.ID)
	return 0
}

// promptPassword reads a single line from stdin. We do NOT hide the
// echo here — that would require terminal control and extra deps; the
// CLI's primary users are operators piping via `echo | dockery-api`
// or interactive shells over SSH where they accept visible input.
// M4 may add a terminal-aware hidden prompt.
func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func atoi(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
