FROM node:20.20.2-alpine AS build
WORKDIR /app
COPY package.json package-lock.json .npmrc ./
RUN npm ci --no-fund --no-audit
COPY . .
RUN npm run build

# Root olmayan (uid 101) nginx; 8080'de dinler
FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --retries=12 CMD wget -q -O /dev/null http://127.0.0.1:8080/ || exit 1
