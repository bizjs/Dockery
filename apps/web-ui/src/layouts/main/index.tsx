/**
 * Main Layout Component
 * Application layout with header, main content area, and footer
 */

import { Outlet, Link } from 'react-router-dom';
import { Package, ExternalLink } from 'lucide-react';

interface MainLayoutProps {
  title?: string;
  registryUrl?: string;
  version?: string;
  showLinks?: boolean;
}

/**
 * Main Layout Component
 */
export function MainLayout({
  title = 'Light Registry',
  registryUrl,
  version = '0.0.0',
  showLinks = true,
}: MainLayoutProps = {}) {
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
            <div className="flex items-center gap-4">{/* Theme toggle, settings, etc. will go here */}</div>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="flex-1 container mx-auto px-4 py-6">
        <Outlet />
      </main>

      {/* Footer */}
      <footer className="bg-footer-background text-footer-text border-t border-border">
        <div className="container mx-auto px-4 py-6">
          <div className="flex flex-col md:flex-row items-center justify-between gap-4">
            {/* Version Info */}
            <div className="flex items-center gap-2 text-sm">
              <span className="text-footer-neutral-text">{title}</span>
              <span className="text-footer-neutral-text">•</span>
              <span className="font-mono">v{version}</span>
            </div>

            {/* Links */}
            {showLinks && (
              <div className="flex items-center gap-6">
                <a
                  href="https://github.com/bizjs/light-registry"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-2 text-sm hover:text-footer-text transition-colors"
                >
                  <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
                    <path
                      fillRule="evenodd"
                      d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"
                      clipRule="evenodd"
                    />
                  </svg>
                  <span>GitHub</span>
                  <ExternalLink className="h-3 w-3" />
                </a>
                <a
                  href="https://github.com/bizjs/light-registry/blob/main/README.md"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-2 text-sm hover:text-footer-text transition-colors"
                >
                  <span>Documentation</span>
                  <ExternalLink className="h-3 w-3" />
                </a>
              </div>
            )}
          </div>

          {/* Copyright */}
          <div className="mt-4 text-center text-xs text-footer-neutral-text">
            <p>
              Made with ❤️ by{' '}
              <a
                href="https://github.com/hstarorg"
                target="_blank"
                rel="noopener noreferrer"
                className="hover:text-footer-text transition-colors"
              >
                hstarorg
              </a>
            </p>
          </div>
        </div>
      </footer>
    </div>
  );
}
