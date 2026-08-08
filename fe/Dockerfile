FROM node:22-alpine AS builder

WORKDIR /app

COPY fe/package.json fe/pnpm-lock.yaml ./
RUN corepack enable \
 && corepack prepare pnpm@9.15.0 --activate \
 && pnpm install --frozen-lockfile

COPY fe/ ./
RUN pnpm build

FROM nginx:1.29-alpine

COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80
