const API_INTERNAL = process.env.OPENBASE_API_INTERNAL_URL ?? "http://localhost:8080";

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  eslint: { ignoreDuringBuilds: true },
  async rewrites() {
    return [
      {
        source: "/v1/:path*",
        destination: `${API_INTERNAL}/v1/:path*`,
      },
      // Liveness/readiness probes live outside /v1/* on the Go API, so the
      // same-origin health check in ApiStatus would 404 on Next itself and
      // pin the status bar to "Degraded". Proxy them too.
      {
        source: "/healthz",
        destination: `${API_INTERNAL}/healthz`,
      },
      {
        source: "/readyz",
        destination: `${API_INTERNAL}/readyz`,
      },
    ];
  },
};

export default nextConfig;