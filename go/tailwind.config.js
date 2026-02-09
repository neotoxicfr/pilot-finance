/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./templates/**/*.html",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        background: 'var(--background)',
        foreground: 'var(--foreground)',
        border: 'var(--border)',
        accent: 'var(--accent)',
        'muted-foreground': 'var(--muted-foreground)',
      }
    }
  },
  plugins: [],
}
