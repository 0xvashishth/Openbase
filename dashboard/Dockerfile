# Build stage
FROM node:22-alpine AS build
ARG OPENBASE_API_INTERNAL_URL=http://localhost:8080
ENV OPENBASE_API_INTERNAL_URL=$OPENBASE_API_INTERNAL_URL
WORKDIR /app
COPY package*.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

# Runtime stage (uses Next.js standalone output to keep the image small)
FROM node:22-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
EXPOSE 3000
ENV PORT=3000 HOSTNAME=0.0.0.0
CMD ["node", "server.js"]