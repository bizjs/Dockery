/**
 * TagTable Component
 * Table displaying Docker image tags with details
 */

import type { ReactNode } from 'react'
import { Table, TableBody, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Checkbox } from '@/components/ui/checkbox'
import { ArrowUpDown, ArrowUp, ArrowDown } from 'lucide-react'
import { Button } from '@/components/ui/button'

type SortField = 'tag' | 'size' | 'created'
type SortDirection = 'asc' | 'desc'

interface TagTableProps {
  children: ReactNode
  sortField: SortField | null
  sortDirection: SortDirection
  onSort: (field: SortField) => void
  // Selection header: only rendered when selection is available (admin/write).
  showSelectionColumn?: boolean
  allOnPageSelected?: boolean
  someOnPageSelected?: boolean
  onToggleSelectPage?: () => void
}

export function TagTable({
  children,
  sortField,
  sortDirection,
  onSort,
  showSelectionColumn = false,
  allOnPageSelected = false,
  someOnPageSelected = false,
  onToggleSelectPage,
}: TagTableProps) {
  const getSortIcon = (field: SortField) => {
    if (sortField !== field) {
      return <ArrowUpDown className="ml-2 h-4 w-4" />
    }
    return sortDirection === 'asc' ? (
      <ArrowUp className="ml-2 h-4 w-4" />
    ) : (
      <ArrowDown className="ml-2 h-4 w-4" />
    )
  }

  return (
    <div className="border rounded-lg">
      <Table>
        <TableHeader>
          <TableRow>
            {showSelectionColumn && (
              <TableHead className="w-10">
                <Checkbox
                  checked={allOnPageSelected ? true : someOnPageSelected ? 'indeterminate' : false}
                  onCheckedChange={() => onToggleSelectPage?.()}
                  aria-label="Select all on this page"
                />
              </TableHead>
            )}
            <TableHead>
              <Button
                variant="ghost"
                onClick={() => onSort('tag')}
                className="h-8 px-2 lg:px-3"
              >
                Tag
                {getSortIcon('tag')}
              </Button>
            </TableHead>
            <TableHead>
              <Button
                variant="ghost"
                onClick={() => onSort('size')}
                className="h-8 px-2 lg:px-3"
              >
                Size
                {getSortIcon('size')}
              </Button>
            </TableHead>
            <TableHead>
              <Button
                variant="ghost"
                onClick={() => onSort('created')}
                className="h-8 px-2 lg:px-3"
              >
                Created
                {getSortIcon('created')}
              </Button>
            </TableHead>
            <TableHead>Digest</TableHead>
            <TableHead>Architecture</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>{children}</TableBody>
      </Table>
    </div>
  )
}
