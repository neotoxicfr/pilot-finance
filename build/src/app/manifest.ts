import { MetadataRoute } from 'next';

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: 'Pilot Finance',
    short_name: 'Pilot',
    description: 'Dashboard financier personnel',
    start_url: '/',
    display: 'standalone', // C'est ça qui cache la barre d'URL
    background_color: '#020617',
    theme_color: '#020617',
    icons: [
      {
        src: '/apple-icon.png',
        sizes: '192x192',
        type: 'image/png',
      },
      {
        src: '/logo.svg',
        sizes: 'any',
        type: 'image/svg+xml',
      }
    ],
  }
}