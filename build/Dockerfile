FROM node:22-alpine AS deps

RUN apk add --no-cache libc6-compat python3 make g++

WORKDIR /app

COPY package.json package-lock.json* ./

RUN npm install

FROM node:22-alpine AS builder

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED 1

RUN npm run build

FROM node:22-alpine AS runner

WORKDIR /app

ENV NODE_ENV production
ENV NEXT_TELEMETRY_DISABLED 1
ENV PORT 3000
ENV DATABASE_URL="file:/data/pilot.db"
ENV HOSTNAME "0.0.0.0"

RUN apk add --no-cache sqlite
RUN mkdir -p /data && chown node:node /data

COPY --from=builder --chown=node:node /app/.next/standalone ./
COPY --from=builder --chown=node:node /app/.next/static ./.next/static
COPY --from=builder --chown=node:node /app/public ./public
COPY --from=builder --chown=node:node /app/drizzle.config.ts ./drizzle.config.ts
COPY --from=builder --chown=node:node /app/package.json ./package.json
COPY --from=builder --chown=node:node /app/entrypoint.sh ./entrypoint.sh

RUN chmod +x ./entrypoint.sh

USER node

EXPOSE 3000

ENTRYPOINT ["./entrypoint.sh"]