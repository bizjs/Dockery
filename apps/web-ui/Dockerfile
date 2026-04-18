# Build stage
FROM node:24-alpine AS builder

WORKDIR /app

# Copy package files
COPY package.json pnpm-lock.yaml ./

# Install pnpm
RUN npm install -g pnpm

# Install dependencies
RUN pnpm install --frozen-lockfile

# Copy source code
COPY . .

# Build the application
RUN pnpm run build

# Production stage
FROM nginx:alpine

# Copy built files to nginx
COPY --from=builder /app/dist /usr/share/nginx/html

# Copy nginx configuration template
COPY nginx.conf.template /etc/nginx/templates/default.conf.template

# Set default environment variable
ENV REGISTRY_URL=http://registry:5000

EXPOSE 80

# nginx will automatically process templates in /etc/nginx/templates/
# and substitute environment variables using envsubst
CMD ["nginx", "-g", "daemon off;"]
