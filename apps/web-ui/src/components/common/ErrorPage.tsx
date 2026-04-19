/**
 * ErrorPage Component
 * Display error messages and recovery options
 */

import { AlertCircle, RefreshCw, Home } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

interface ErrorPageProps {
  title?: string
  message?: string
  code?: string
  onRetry?: () => void
  onGoHome?: () => void
}

export function ErrorPage({
  title = 'Something went wrong',
  message = 'An unexpected error occurred',
  code,
  onRetry,
  onGoHome,
}: ErrorPageProps) {
  return (
    <div className="flex items-center justify-center min-h-[400px] p-4">
      <Card className="max-w-md w-full">
        <CardHeader>
          <div className="flex items-center gap-3">
            <AlertCircle className="h-8 w-8 text-destructive" />
            <div>
              <CardTitle>{title}</CardTitle>
              {code && <CardDescription className="font-mono text-xs mt-1">{code}</CardDescription>}
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">{message}</p>
          
          <div className="flex gap-2">
            {onRetry && (
              <Button onClick={onRetry} variant="default" className="flex-1">
                <RefreshCw className="h-4 w-4 mr-2" />
                Retry
              </Button>
            )}
            {onGoHome && (
              <Button onClick={onGoHome} variant="outline" className="flex-1">
                <Home className="h-4 w-4 mr-2" />
                Go Home
              </Button>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
