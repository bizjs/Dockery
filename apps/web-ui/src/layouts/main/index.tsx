/**
 * Main Layout Component
 * Application layout with header, main content area, and compact footer.
 */

import { Outlet, Link } from 'react-router-dom';
import { Package, ExternalLink } from 'lucide-react';

import { UserMenu } from './UserMenu';

// __APP_VERSION__ is injected by vite.config.ts at build time.
const APP_VERSION = __APP_VERSION__;

interface MainLayoutProps {
  title?: string;
  registryUrl?: string;
}

/**
 * Main Layout Component
 */
export function MainLayout({ title = 'Dockery', registryUrl }: MainLayoutProps = {}) {
  return (
    <div className="flex flex-col min-h-screen bg-background">
      {/* Header */}
      <header className="bg-header-background text-header-text">
        <div className="container mx-auto px-4 py-4">
          <div className="flex items-center justify-between">
            {/* Logo and Title */}
            <Link to="/" className="flex items-center gap-3 hover:opacity-80 transition-opacity">
              <Package className="h-8 w-8" />
              <div>
                <h1 className="text-xl font-bold">{title}</h1>
                {registryUrl && <p className="text-sm text-header-accent-text">{registryUrl}</p>}
              </div>
            </Link>

            {/* Navigation Actions */}
            <div className="flex items-center gap-4">
              <UserMenu />
            </div>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="flex-1 container mx-auto px-4 py-6">
        <Outlet />
      </main>

      {/* Compact footer — single row on md+, wraps on narrow screens */}
      <footer className="bg-footer-background text-footer-text border-t border-border">
        <div className="container mx-auto px-4 py-2.5 flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-xs">
          <div className="flex items-center gap-2">
            <span className="text-footer-neutral-text">{title}</span>
            <span className="font-mono">v{APP_VERSION}</span>
            <span className="text-footer-neutral-text">·</span>
            <span className="text-footer-neutral-text">
              by{' '}
              <a
                href="https://github.com/hstarorg"
                target="_blank"
                rel="noopener noreferrer"
                className="hover:text-footer-text transition-colors"
              >
                hstarorg
              </a>
            </span>
          </div>
          <div className="flex items-center gap-4">
            <a
              href="https://github.com/bizjs/light-registry"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-1 hover:text-footer-text transition-colors"
            >
              <span>GitHub</span>
              <ExternalLink className="h-3 w-3" />
            </a>
            <a
              href="https://github.com/bizjs/light-registry/blob/main/docs/deployment.md"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-1 hover:text-footer-text transition-colors"
            >
              <span>Docs</span>
              <ExternalLink className="h-3 w-3" />
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}
