/**
 * VersionNotification Component
 * Notify users about new versions
 */

import { Info, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'

interface VersionNotificationProps {
  currentVersion: string
  latestVersion: string
  onDismiss: () => void
}

export function VersionNotification({
  currentVersion,
  latestVersion,
  onDismiss,
}: VersionNotificationProps) {
  return (
    <Card className="p-4 mb-4 bg-accent">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <Info className="h-5 w-5 text-accent-text mt-0.5" />
          <div>
            <p className="font-medium text-sm">New version available</p>
            <p className="text-sm text-muted-foreground mt-1">
              Version {latestVersion} is available. You are currently using {currentVersion}.
            </p>
          </div>
        </div>
        <Button variant="ghost" size="sm" onClick={onDismiss} className="h-8 w-8 p-0">
          <X className="h-4 w-4" />
        </Button>
      </div>
    </Card>
  )
}
