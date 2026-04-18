/**
 * RegistryAuthentication Component
 * Handle Docker registry authentication
 */

import { Lock } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useState } from 'react'

interface RegistryAuthenticationProps {
  onAuthenticate: (username: string, password: string) => void
  registryUrl: string
}

export function RegistryAuthentication({ onAuthenticate, registryUrl }: RegistryAuthenticationProps) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    onAuthenticate(username, password)
  }

  return (
    <div className="flex items-center justify-center min-h-100 p-4">
      <Card className="max-w-md w-full">
        <CardHeader>
          <div className="flex items-center gap-3">
            <Lock className="h-6 w-6" />
            <div>
              <CardTitle>Authentication Required</CardTitle>
              <CardDescription className="mt-1">{registryUrl}</CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="username" className="text-sm font-medium">Username</label>
              <Input
                id="username"
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="Enter username"
                required
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="password" className="text-sm font-medium">Password</label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter password"
                required
              />
            </div>
            <Button type="submit" className="w-full">
              Authenticate
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
