# Yearning 前端镜像：Vite 构建 + Nginx 托管
# docker build -f docker/frontend.Dockerfile -t yearning-frontend:dev .   （上下文为仓库根目录）

# ---------- 阶段一：构建前端 ----------
FROM node:20-alpine AS build
ARG NPM_REGISTRY=https://registry.npmmirror.com
WORKDIR /src

# 先装依赖，命中层缓存
COPY front/package.json front/pnpm-lock.yaml front/.npmrc ./
RUN corepack enable \
 && pnpm install --frozen-lockfile --registry="$NPM_REGISTRY"

COPY front/ ./
RUN pnpm build

# ---------- 阶段二：Nginx 托管 ----------
FROM nginx:1.27-alpine AS run

COPY docker/nginx/frontend.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
