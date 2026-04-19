# Dockery - Web UI

Modern React-based web interface for Docker Registry, built with TypeScript, Tailwind CSS, and shadcn/ui.

## 🚀 Features

- 📦 Browse Docker images and tags
- 🔍 Search and filter functionality
- 🗑️ Delete images and tags
- 📊 View image history and details
- 🎨 Light/Dark theme support
- 📱 Fully responsive design
- ⚡ Fast and modern UI with React 19
- 🔒 Authentication support

## 🛠️ Tech Stack

- **React 19** - UI framework
- **TypeScript** - Type safety
- **Tailwind CSS 4** - Styling with oklch colors
- **shadcn/ui** - Component library
- **React Router v6** - Routing
- **Vite** - Build tool with rolldown
- **Vitest** - Testing framework

## 📦 Installation

```bash
# Install dependencies
pnpm install

# Start development server
pnpm dev

# Build for production
pnpm build

# Preview production build
pnpm preview

# Run tests
pnpm test
```

## ⚙️ Configuration

Create a `.env` file based on `.env.example`:

```env
VITE_REGISTRY_URL=http://localhost:5000
VITE_CATALOG_ELEMENTS_LIMIT=100
VITE_USE_CONTROL_CACHE_HEADER=false
VITE_SINGLE_REGISTRY=false
VITE_SHOW_CONTENT_DIGEST=true
VITE_SHOW_CATALOG_NB_TAGS=true
```

## 🎨 Components

### Core Components

- **Catalog** - Browse Docker images
- **TagList** - View image tags
- **TagHistory** - View tag history
- **Dialogs** - Manage registries and images

## 🔧 Development

```bash
# Run development server
pnpm dev

# Linting
pnpm lint
```

## 🤝 Contributing

Contributions are welcome! Please read the contributing guidelines first.
