/**
 * Skyscape - Core JavaScript
 * HTMX Integration, Service Worker, PWA, and Notifications
 */

(function() {
  'use strict';

  // Initialize global namespace
  window.Skyscape = window.Skyscape || {};

  // ============================================
  // HTMX Integration
  // ============================================

  // Registry for page initializers - functions that run on page load/swap
  const pageInitializers = new Map();

  /**
   * Register a function to run when a page loads (initial or HTMX swap)
   * @param {string} selector - CSS selector to match (e.g., '[data-page="messages"]')
   * @param {Function} fn - Initializer function, receives the matched element
   */
  window.Skyscape.onPage = function(selector, fn) {
    pageInitializers.set(selector, fn);
  };

  /**
   * Run all matching page initializers for the given root element
   */
  function runPageInitializers(root = document) {
    pageInitializers.forEach((fn, selector) => {
      const elements = root.querySelectorAll(selector);
      elements.forEach(el => {
        try {
          fn(el);
        } catch (err) {
          console.error(`[Skyscape] Initializer error for ${selector}:`, err);
        }
      });
    });
  }

  // Run initializers on initial page load
  document.addEventListener('DOMContentLoaded', () => runPageInitializers());

  // Run initializers after HTMX swaps content
  document.addEventListener('htmx:afterSettle', (event) => {
    runPageInitializers(event.detail.target);
  });

  /**
   * Helper to run code once per element (prevents double-init on HTMX swaps)
   */
  window.Skyscape.initOnce = function(element, key, fn) {
    const cache = element._skyscapeInit = element._skyscapeInit || {};
    if (cache[key]) return;
    cache[key] = true;
    fn(element);
  };

  /**
   * Show a toast notification (works with HTMX pages)
   */
  window.Skyscape.toast = function(message, type = 'info') {
    const toast = document.createElement('div');
    toast.className = 'toast toast-end z-[100]';
    toast.innerHTML = `<div class="alert alert-${type}"><span>${message}</span></div>`;
    document.body.appendChild(toast);
    setTimeout(() => toast.remove(), 4000);
  };

  // ============================================
  // Service Worker Registration
  // ============================================

  if ('serviceWorker' in navigator) {
    // Register SW once on initial load
    navigator.serviceWorker.register('/sw.js', { scope: '/' })
      .then((registration) => {
        console.log('[PWA] Service Worker registered:', registration.scope);

        // Check for updates immediately and every 60 seconds
        registration.update();
        setInterval(() => registration.update(), 60000);

        // Listen for new service worker
        registration.addEventListener('updatefound', () => {
          const newWorker = registration.installing;
          console.log('[PWA] New Service Worker installing...');

          newWorker.addEventListener('statechange', () => {
            if (newWorker.state === 'installed' && navigator.serviceWorker.controller) {
              console.log('[PWA] New version available, reloading...');
              window.location.reload();
            }
          });
        });
      })
      .catch((error) => {
        console.error('[PWA] Service Worker registration failed:', error);
      });

    // Reload when new service worker takes control
    navigator.serviceWorker.addEventListener('controllerchange', () => {
      console.log('[PWA] Controller changed, reloading...');
      window.location.reload();
    });
  }

  // ============================================
  // PWA Install Prompt
  // ============================================

  window.Skyscape.deferredPrompt = null;
  window.Skyscape.isInstalled = window.matchMedia('(display-mode: standalone)').matches || window.navigator.standalone;

  // Capture install prompt event (Chrome/Edge/Android)
  window.addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault();
    window.Skyscape.deferredPrompt = e;
    if (!window.Skyscape.isInstalled && !localStorage.getItem('pwa-dismissed')) {
      showInstallBanner();
    }
  });

  // Detect iOS
  const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent) && !window.MSStream;

  // Show install banner on iOS if not installed (delayed to not interrupt)
  if (isIOS && !window.Skyscape.isInstalled && !localStorage.getItem('pwa-dismissed')) {
    setTimeout(showIOSInstallBanner, 3000);
  }

  function showInstallBanner() {
    if (document.getElementById('pwa-install-banner')) return;

    const banner = document.createElement('div');
    banner.id = 'pwa-install-banner';
    banner.className = 'fixed bottom-20 md:bottom-4 left-4 right-4 md:left-auto md:right-4 md:w-80 bg-base-100 border border-base-300 rounded-xl shadow-xl p-4 z-50';
    banner.innerHTML = `
      <div class="flex items-start gap-3">
        <img src="/public/logo.svg" class="w-12 h-12 rounded-lg" alt="Skyscape">
        <div class="flex-1 min-w-0">
          <h3 class="font-bold text-sm">Install Skyscape</h3>
          <p class="text-xs opacity-70">Add to your home screen for quick access</p>
        </div>
        <button onclick="window.Skyscape.dismissInstallBanner()" class="btn btn-ghost btn-xs btn-circle">✕</button>
      </div>
      <div class="flex gap-2 mt-3">
        <button onclick="window.Skyscape.installPWA()" class="btn btn-primary btn-sm flex-1">Install</button>
        <button onclick="window.Skyscape.dismissInstallBanner()" class="btn btn-ghost btn-sm">Not now</button>
      </div>
    `;
    document.body.appendChild(banner);
  }

  function showIOSInstallBanner() {
    if (document.getElementById('pwa-install-banner')) return;

    const banner = document.createElement('div');
    banner.id = 'pwa-install-banner';
    banner.className = 'fixed bottom-20 md:bottom-4 left-4 right-4 md:left-auto md:right-4 md:w-80 bg-base-100 border border-base-300 rounded-xl shadow-xl p-4 z-50';
    banner.innerHTML = `
      <div class="flex items-start gap-3">
        <img src="/public/logo.svg" class="w-12 h-12 rounded-lg" alt="Skyscape">
        <div class="flex-1 min-w-0">
          <h3 class="font-bold text-sm">Install Skyscape</h3>
          <p class="text-xs opacity-70">Add to your home screen for quick access</p>
        </div>
        <button onclick="window.Skyscape.dismissInstallBanner()" class="btn btn-ghost btn-xs btn-circle">✕</button>
      </div>
      <div class="mt-3 space-y-2">
        <div class="flex items-center gap-3 text-sm">
          <span class="badge badge-primary badge-sm">1</span>
          <span>Tap <svg class="w-5 h-5 inline text-primary" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M9 8.25H7.5a2.25 2.25 0 00-2.25 2.25v9a2.25 2.25 0 002.25 2.25h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25H15m0-3l-3-3m0 0l-3 3m3-3V15" /></svg> in Safari toolbar below</span>
        </div>
        <div class="flex items-center gap-3 text-sm">
          <span class="badge badge-primary badge-sm">2</span>
          <span>Scroll and tap <strong>"Add to Home Screen"</strong></span>
        </div>
      </div>
      <button onclick="window.Skyscape.dismissInstallBanner()" class="btn btn-ghost btn-sm btn-block mt-3">Maybe later</button>
    `;
    document.body.appendChild(banner);

    // Add a pulsing indicator pointing to Safari's share button
    const indicator = document.createElement('div');
    indicator.id = 'pwa-share-indicator';
    indicator.className = 'fixed bottom-2 left-1/2 -translate-x-1/2 z-50';
    indicator.innerHTML = `
      <div class="animate-bounce text-primary">
        <svg class="w-8 h-8" fill="currentColor" viewBox="0 0 24 24"><path d="M12 16l-6-6h12l-6 6z"/></svg>
      </div>
    `;
    document.body.appendChild(indicator);
  }

  window.Skyscape.installPWA = function() {
    if (window.Skyscape.deferredPrompt) {
      window.Skyscape.deferredPrompt.prompt();
      window.Skyscape.deferredPrompt.userChoice.then((result) => {
        if (result.outcome === 'accepted') {
          console.log('[PWA] App installed');
        }
        window.Skyscape.deferredPrompt = null;
        window.Skyscape.dismissInstallBanner();
      });
    }
  };

  window.Skyscape.dismissInstallBanner = function() {
    const banner = document.getElementById('pwa-install-banner');
    if (banner) banner.remove();
    const indicator = document.getElementById('pwa-share-indicator');
    if (indicator) indicator.remove();
    localStorage.setItem('pwa-dismissed', Date.now());
  };

  // ============================================
  // Notifications
  // ============================================

  window.Skyscape.requestNotifications = async function() {
    if (!('Notification' in window)) {
      console.warn('[Notifications] Not supported');
      return false;
    }

    if (Notification.permission === 'granted') {
      return true;
    }

    if (Notification.permission !== 'denied') {
      const permission = await Notification.requestPermission();
      return permission === 'granted';
    }

    return false;
  };

  window.Skyscape.subscribeToPush = async function(vapidPublicKey) {
    const registration = await navigator.serviceWorker.ready;

    const subscription = await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: vapidPublicKey
    });

    return subscription;
  };

  window.Skyscape.notify = async function(title, options = {}) {
    if (Notification.permission !== 'granted') {
      console.warn('[Notifications] Permission not granted');
      return;
    }

    try {
      const registration = await navigator.serviceWorker.ready;
      await registration.showNotification(title, {
        icon: '/public/logo.svg',
        badge: '/public/logo.svg',
        vibrate: [200, 100, 200, 100, 200],
        requireInteraction: true,
        renotify: true,
        silent: false,
        tag: 'skyscape-' + Date.now(),
        ...options
      });
      console.log('[Notifications] Shown:', title);
    } catch (err) {
      console.error('[Notifications] Failed:', err);
    }
  };

})();
