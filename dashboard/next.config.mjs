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
    ];
  },
};

export default nextConfig;