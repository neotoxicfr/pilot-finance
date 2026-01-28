/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',

  experimental: {
    optimizePackageImports: [
      'lucide-react',
      'recharts',
      'date-fns',
      '@simplewebauthn/browser',
    ],
  },

  serverExternalPackages: ['argon2', 'better-sqlite3', 'pino'],

  images: {
    formats: ['image/avif', 'image/webp'],
    minimumCacheTTL: 31536000,
  },

  compress: true,

  poweredByHeader: false,

  logging: {
    fetches: {
      fullUrl: process.env.NODE_ENV === 'development',
    },
  },
};

// Only enable bundle analyzer when explicitly requested via ANALYZE=true
// This keeps @next/bundle-analyzer in devDependencies without breaking production builds
export default process.env.ANALYZE === 'true'
  ? (await import('@next/bundle-analyzer')).default({
      enabled: true,
    })(nextConfig)
  : nextConfig;
