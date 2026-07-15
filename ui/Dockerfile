FROM node:22-alpine AS builder

WORKDIR /app
COPY package.json package-lock.json svelte.config.js vite.config.ts tsconfig.json ./
RUN npm ci --ignore-scripts
COPY . .
RUN npm run prepare
RUN npm run build

FROM node:22-alpine

WORKDIR /app
COPY --from=builder /app/build ./build
COPY --from=builder /app/package.json /app/package-lock.json ./
RUN npm ci --omit=dev --ignore-scripts

ENV HOST=0.0.0.0
ENV NODE_ENV=production
ENV PORT=3000
EXPOSE 3000

RUN addgroup -g 1001 -S semsource && adduser -S semsource -u 1001 -G semsource
USER semsource

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:3000/ || exit 1

CMD ["node", "build"]
