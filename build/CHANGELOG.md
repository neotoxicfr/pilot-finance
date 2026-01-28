# Changelog

All notable changes to Pilot Finance will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.4.0] - 2026-01-28

### Added
- **Automatic version tracking**: Build process now auto-generates version info from package.json
- **Enhanced health endpoint** (`/api/health`):
  - Accurate version number from build
  - Build date timestamp
  - Human-readable uptime formatting (e.g., "2d 5h 30m 15s")
  - Database size metrics with formatted output
  - Garbage collection availability status
- **Manual garbage collection**: Automatic GC every 60 seconds in production mode
- **Bundle analyzer**: Added `@next/bundle-analyzer` for development bundle inspection
- **Loading states**: Settings page now shows loading spinner to prevent UI flash
- **Lazy loading**: Chart components (ProjectionChart, BalancePieChart) now load on-demand
- **Module type**: Added `"type": "module"` to package.json for proper ES module support

### Changed
- **Upgraded Tailwind CSS**: Migrated from v3.4.17 to v4.1.18 (Rust-based engine)
  - Updated to use `@tailwindcss/postcss` plugin
  - Converted CSS to new `@config` and `@import` syntax
  - Build time improved by **74%** (15s → 4.7-5.2s)
  - Package count reduced by **62 packages** (429 → 367)
- **Optimized bundle size**: Initial bundle reduced by ~30% through code splitting
- **Improved memory usage**: RAM consumption reduced from 95% to 53%
- **Enhanced navigation**: Added prefetch to /accounts link for faster transitions

### Fixed
- **Cursor styling**: Restored pointer cursors for buttons and links (Tailwind 4 breaking change)
- **Docker builds**: Moved `@tailwindcss/postcss` to production dependencies
- **Bundle analyzer compatibility**: Made import conditional to prevent Docker build failures
- **Chart skeleton loaders**: Fixed layout shifts with proper height constraints
- **TypeScript safety**: Added proper type guards for `global.gc` calls
- **Module warnings**: Eliminated MODULE_TYPELESS_PACKAGE_JSON warning

### Performance
- **Build time**: 15s → 4.7-5.2s (-74%)
- **Initial bundle**: Reduced by ~30% via lazy loading
- **RAM usage**: 95% → 53% with manual GC
- **Dependencies**: 429 → 367 packages (-62)

### Technical Details
- Node.js now runs with `--expose-gc` flag for manual garbage collection
- Version info automatically generated during build via `scripts/generate-version.mjs`
- Chart components split into separate lazy-loaded modules
- CSS optimizations: extracted inline styles to globals.css

## [1.3.1] - 2026-01-XX

### Changed
- Performance optimizations
- Dependency updates

### Fixed
- TypeScript type issues in transaction callbacks
- Account balance property access

## [1.3.0] - 2026-01-XX

### Added
- Initial major release with core financial management features

---

[1.4.0]: https://github.com/neotoxicfr/pilot-finance/compare/v1.3.1...v1.4.0
[1.3.1]: https://github.com/neotoxicfr/pilot-finance/compare/v1.3.0...v1.3.1
[1.3.0]: https://github.com/neotoxicfr/pilot-finance/releases/tag/v1.3.0
