import { useState } from 'react';

export function useAuthentication() {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [credentials, setCredentials] = useState<{ username: string; password: string } | null>(null);

  const login = (username: string, password: string) => {
    setCredentials({ username, password });
    setIsAuthenticated(true);
  };

  const logout = () => {
    setCredentials(null);
    setIsAuthenticated(false);
  };

  return { isAuthenticated, credentials, login, logout };
}
