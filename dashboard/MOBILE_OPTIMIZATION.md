# Mobile Optimization Plan for Openbase Dashboard

## Executive Summary

Transform the Openbase dashboard into a **first-class mobile experience** that feels native when installed as a PWA on Android. Desktop experience remains **unchanged**.

---

## Current State Analysis

### Existing Mobile Support
- ✅ Responsive sidebar via `Sheet` component (slide-in drawer on `<md`)
- ✅ Topbar wraps on mobile (`flex-wrap`)
- ✅ StatusBar hides API status on small screens (`min-[560px]:hidden`)
- ✅ PageShell has responsive padding (`px-4 sm:px-6`)
- ✅ Sidebar collapse/expand with tooltip labels on rail mode

### Gaps for Native App Feel
| Area | Current | Needed for Native Feel |
|------|---------|------------------------|
| **Installability** | No manifest, no SW | PWA manifest + Service Worker |
| **Viewport** | Default | `viewport-fit=cover`, theme-color |
| **Safe Areas** | None | `env(safe-area-inset-*)` support |
| **Bottom Nav** | None | Primary navigation pattern |
| **Topbar Height** | 56px (min-h-14) | ~48px compact, collapsible search |
| **StatusBar** | 32px (h-8) always visible | Hidden on project pages, merged |
| **Sidebar Drawer** | Basic Sheet | Native-feeling with swipe, backdrop |
| **Touch Targets** | 36-40px | Minimum 48×48px (Material) / 44×44px (iOS) |
| **Gestures** | None | Swipe-back, pull-to-refresh, edge swipe |
| **Transitions** | Basic slide | Platform-appropriate spring curves |
| **Haptics** | None | Light/medium/heavy feedback |

---

## Design Principles

### 1. **Desktop Unchanged**
- All `md:` and `lg:` breakpoints preserve current behavior
- No visual regression on desktop

### 2. **Mobile-First Progressive Enhancement**
- Base styles target mobile (< 768px)
- Desktop enhancements layered via `md:`

### 3. **Native App Patterns**
- **Bottom navigation** for 3-5 top-level destinations
- **Floating action button** for primary actions
- **Modal sheets** for secondary flows (not full-page)
- **Swipe gestures** for navigation
- **Safe area respect** for notches, home indicators

### 4. **Performance Budget**
- Initial JS < 170KB gzipped
- LCP < 2.5s on 3G
- CLS < 0.1
- 60fps animations

---

## Architecture

### New Component Structure

```
components/layout/
├── mobile/
│   ├── MobileTopbar.tsx        # Compact header with hamburger + actions
│   ├── MobileBottomNav.tsx     # Primary navigation (3-5 items)
│   ├── MobileStatusBar.tsx     # Merged status, hidden on scroll
│   ├── MobileSheet.tsx         # Native-feeling drawer (swipeable)
│   ├── MobilePageShell.tsx     # Content container with safe insets
│   ├── useMobile.ts            # Hooks: useIsMobile, useSafeArea, useGestures
│   └── index.ts
├── AppShell.tsx                # Updated to compose mobile + desktop
├── PageShell.tsx               # Unchanged (desktop)
└── ...existing files
```

### Breakpoint Strategy

```css
/* Tailwind defaults */
sm:  640px   /* Large phones */
md:  768px   /* Tablets — DESKTOP LAYOUT STARTS HERE */
lg:  1024px  /* Laptops */
xl:  1280px  /* Desktops */
2xl: 1536px  /* Large desktops */
```

**Key decision**: `md` (768px) = tablet = desktop layout. Mobile = `<md`.

---

## Detailed Component Specs

### 1. PWA Manifest (`public/manifest.json`)

```json
{
  "name": "Openbase",
  "short_name": "Openbase",
  "description": "Open-source, self-hosted backend platform",
  "start_url": "/orgs",
  "display": "standalone",
  "orientation": "portrait-primary",
  "background_color": "#08090a",
  "theme_color": "#08090a",
  "icons": [
    { "src": "/icons/icon-72.png", "sizes": "72x72", "type": "image/png", "purpose": "any maskable" },
    { "src": "/icons/icon-96.png", "sizes": "96x96", "type": "image/png", "purpose": "any maskable" },
    { "src": "/icons/icon-128.png", "sizes": "128x128", "type": "image/png", "purpose": "any maskable" },
    { "src": "/icons/icon-144.png", "sizes": "144x144", "type": "image/png", "purpose": "any maskable" },
    { "src": "/icons/icon-152.png", "sizes": "152x152", "type": "image/png", "purpose": "any maskable" },
    { "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any maskable" },
    { "src": "/icons/icon-384.png", "sizes": "384x384", "type": "image/png", "purpose": "any maskable" },
    { "src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any maskable" }
  ],
  "categories": ["developer", "productivity"],
  "shortcuts": [
    { "name": "Organizations", "url": "/orgs", "icons": [{ "src": "/icons/orgs.png", "sizes": "96x96" }] },
    { "name": "Projects", "url": "/projects", "icons": [{ "src": "/icons/projects.png", "sizes": "96x96" }] }
  ]
}
```

### 2. Service Worker (`public/sw.js`)

- **Strategy**: Stale-while-revalidate for assets, network-first for API
- **Precache**: Static assets, manifest, offline page
- **Runtime**: Cache API responses with 5-min TTL
- **Offline**: Show cached data + "You're offline" banner

### 3. MobileTopbar (`components/layout/mobile/MobileTopbar.tsx`)

**Height**: 48px (vs 56px desktop)

**Structure**:
```
┌─────────────────────────────────────────────┐
│ ☰  Openbase              🔍  👤  │ 48px    │
└─────────────────────────────────────────────┘
```

- **Left**: Hamburger menu (opens MobileSheet)
- **Center**: Brand + context badge (org/project name, truncated)
- **Right**: Search trigger (opens full-screen search modal), Theme toggle, User avatar
- **Search**: Tap → full-screen `GlobalSearch` modal (not inline)
- **Scroll behavior**: Hide on scroll down, show on scroll up

### 4. MobileBottomNav (`components/layout/mobile/MobileBottomNav.tsx`)

**Height**: 56px + safe-area-inset-bottom

**Items** (context-aware):

| Scope | Items (max 5) |
|-------|---------------|
| Platform | Organizations, Projects, Settings, Account |
| Org | Projects, Members, Settings, (Back to Orgs) |
| Project | Overview, Tables, SQL, Connect, More (overflow menu) |

**Behavior**:
- Active item highlighted with acid-lime indicator
- Icons only on very small screens (<360px), icons+labels otherwise
- Haptic feedback on tap
- Hide on scroll down in content areas, show on scroll up

### 5. MobileSheet (`components/layout/mobile/MobileSheet.tsx`)

**Improvements over current Sheet**:
- **Swipe from left edge** to open (like native drawer)
- **Swipe right on content** or **backdrop tap** to close
- **Spring animation** (not linear)
- **Backdrop blur** with `bg-void/70 backdrop-blur-sm`
- **Full height** with safe-area-inset-top/bottom
- **Sections**: Navigation, Account, Settings, Sign out
- **Collapsible sections** for org/project tools

### 6. MobilePageShell (`components/layout/mobile/MobilePageShell.tsx`)

```tsx
<div className="flex h-full flex-col">
  {/* Safe area top handled by MobileTopbar */}
  <main className="flex-1 overflow-y-auto pb-[calc(56px+env(safe-area-inset-bottom))] px-4">
    {children}
  </main>
  {/* Bottom nav floats over content */}
</div>
```

- **Padding**: 16px horizontal, bottom padding = bottom nav height + safe area
- **Max-width**: None (full width on mobile)
- **Pull-to-refresh**: Optional, per-page opt-in

### 7. Safe Area Support

**CSS Variables** (in `globals.css`):
```css
:root {
  --safe-top: env(safe-area-inset-top, 0px);
  --safe-right: env(safe-area-inset-right, 0px);
  --safe-bottom: env(safe-area-inset-bottom, 0px);
  --safe-left: env(safe-area-inset-left, 0px);
}
```

**Utility classes**:
```css
.pt-safe { padding-top: var(--safe-top); }
.pb-safe { padding-bottom: var(--safe-bottom); }
.pl-safe { padding-left: var(--safe-left); }
.pr-safe { padding-right: var(--safe-right); }
```

**Applied to**:
- `MobileTopbar` → `pt-safe`
- `MobileBottomNav` → `pb-safe pl-safe pr-safe`
- `MobileSheet` → `pt-safe pb-safe`
- `MobilePageShell` main → `pb-[calc(56px+var(--safe-bottom))]`

---

## Integration Strategy

### AppShell Composition

```tsx
// AppShell.tsx (updated)
export function AppShell({ children }) {
  const isMobile = useIsMobile(); // CSS media query hook
  
  if (isMobile) {
    return (
      <MobileLayout>
        <MobileStatusBar />
        <MobileTopbar />
        <MobilePageShell>{children}</MobilePageShell>
        <MobileBottomNav />
        <MobileSheet />
      </MobileLayout>
    );
  }
  
  // Existing desktop layout (unchanged)
  return <DesktopShell>{children}</DesktopShell>;
}
```

**Key**: `useIsMobile` uses `matchMedia('(max-width: 767px)')` — matches Tailwind's `md` breakpoint exactly.

### Route-Aware Bottom Nav

```tsx
function MobileBottomNav() {
  const route = parseRoute(pathname);
  const items = useMemo(() => getBottomNavItems(route), [route]);
  // ...
}
```

---

## Touch & Gesture Specs

### Touch Targets
- **Minimum**: 48×48px (Material) / 44×44px (iOS HIG)
- **Comfortable**: 56×56px for primary actions
- **Spacing**: 8px between targets

### Gestures
| Gesture | Action | Component |
|---------|--------|-----------|
| Swipe from left edge | Open sidebar | `MobileSheet` |
| Swipe right on content | Close sidebar | `MobileSheet` |
| Tap backdrop | Close sidebar/modal | `MobileSheet`, `MobileSearchModal` |
| Pull down on list | Refresh | `usePullToRefresh` (per-page) |
| Swipe back (iOS) | Navigate back | Browser native |
| Long press | Context menu | Future |

### Haptics (via `navigator.vibrate`)
| Interaction | Pattern |
|-------------|---------|
| Nav tap | `[10]` (light) |
| Button press | `[15]` (light) |
| Destructive action | `[30, 10, 30]` (medium) |
| Error | `[50, 50, 50]` (heavy) |
| Success | `[20, 50, 20]` (medium) |

---

## Implementation Phases

### Phase 1: Foundation (Week 1)
- [ ] PWA manifest + icons
- [ ] Service worker (Workbox via `next-pwa` or custom)
- [ ] Viewport meta + theme-color in `layout.tsx`
- [ ] Safe area CSS variables in `globals.css`
- [ ] `useIsMobile` / `useSafeArea` hooks

### Phase 2: Core Mobile Layout (Week 1-2)
- [ ] `MobileTopbar` (compact, collapsible search)
- [ ] `MobileBottomNav` (context-aware items)
- [ ] `MobilePageShell` (safe insets, bottom padding)
- [ ] Update `AppShell` to compose mobile/desktop
- [ ] Hide desktop `StatusBar`/`Topbar`/`Sidebar` on mobile via CSS

### Phase 3: Native Interactions (Week 2)
- [ ] `MobileSheet` with swipe gestures
- [ ] Full-screen search modal
- [ ] Pull-to-refresh hook
- [ ] Haptic feedback utility
- [ ] Scroll-aware header/bottom-nav hide/show

### Phase 4: Polish (Week 2-3)
- [ ] Touch target audit (all interactive elements ≥48px)
- [ ] Animation spring curves (cubic-bezier(0.4, 0, 0.2, 1))
- [ ] Landscape orientation handling
- [ ] Keyboard avoidance (visual viewport API)
- [ ] Offline banner + cached data indicators
- [ ] Test on real devices (iOS Safari, Chrome Android)

---

## File Changes Summary

### New Files
```
dashboard/
├── public/
│   ├── manifest.json
│   ├── sw.js
│   └── icons/
│       ├── icon-72.png ... icon-512.png
│       ├── orgs.png
│       └── projects.png
├── components/layout/mobile/
│   ├── MobileTopbar.tsx
│   ├── MobileBottomNav.tsx
│   ├── MobileStatusBar.tsx
│   ├── MobileSheet.tsx
│   ├── MobilePageShell.tsx
│   ├── useMobile.ts
│   └── index.ts
├── hooks/
│   ├── useIsMobile.ts
│   ├── useSafeArea.ts
│   ├── useGestures.ts
│   ├── usePullToRefresh.ts
│   └── useHaptics.ts
└── lib/
    └── pwa.ts (registration helper)
```

### Modified Files
```
dashboard/
├── app/layout.tsx              # Add manifest, viewport, SW registration
├── app/globals.css             # Safe area vars, mobile utilities
├── components/layout/AppShell.tsx  # Compose mobile/desktop
├── next.config.mjs             # PWA config (if using next-pwa)
└── package.json                # Add workbox/next-pwa if needed
```

---

## Testing Checklist

### Device Matrix
- [ ] iPhone SE (375px) - smallest common
- [ ] iPhone 14/15 Pro (393px) - Dynamic Island
- [ ] iPhone 14/15 Pro Max (430px)
- [ ] Pixel 7 (412px) - hole punch
- [ ] Galaxy S23 (360px) - small Android
- [ ] iPad Mini (768px) - tablet breakpoint
- [ ] Landscape on all above

### PWA Criteria
- [ ] Install prompt appears
- [ ] Standalone mode (no browser chrome)
- [ ] Splash screen shows
- [ ] Theme color matches
- [ ] Icons sharp on all densities
- [ ] Offline page works
- [ ] Shortcuts work from home screen

### Usability
- [ ] All touch targets ≥48px
- [ ] No horizontal scroll
- [ ] Content readable without zoom
- [ ] Bottom nav accessible (thumb zone)
- [ ] Sheet swipe works both ways
- [ ] Search modal traps focus
- [ ] Safe areas respected
- [ ] No layout shift on load

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Desktop regression | Low | High | Visual regression tests, feature flag |
| PWA caching bugs | Medium | Medium | Versioned cache names, clear on deploy |
| Gesture conflicts | Medium | Low | Passive listeners, `touch-action` CSS |
| Safe area gaps | Low | High | Test on notched devices, fallback 0px |
| Bundle size increase | Low | Medium | Code-split mobile components, tree-shake |

---

## Success Metrics

- **PWA Install Rate**: >15% of mobile visitors
- **Mobile Session Duration**: +20% vs baseline
- **Mobile Task Completion**: >90% for top 5 flows
- **CLS Mobile**: <0.1
- **LCP Mobile 3G**: <3s
- **Zero desktop visual regressions**

---

## Appendix: Design Tokens Reference

### Spacing (mobile)
```css
--space-xs: 4px;   /* 0.25rem */
--space-sm: 8px;   /* 0.5rem */
--space-md: 16px;  /* 1rem */
--space-lg: 24px;  /* 1.5rem */
```

### Touch Targets
```css
--touch-min: 48px;      /* Minimum */
--touch-comfort: 56px;  /* Comfortable */
```

### Z-Index Scale (mobile)
```css
--z-bottom-nav: 100;
--z-topbar: 110;
--z-sheet-backdrop: 120;
--z-sheet: 130;
--z-modal: 140;
--z-toast: 150;
```

---

*Document version: 1.0*  
*Author: UX Engineering*  
*Last updated: 2026-09-07*