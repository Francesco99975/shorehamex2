// vite.config.ts
import tailwindcss from "@tailwindcss/vite";
import fg from "fast-glob";
import { resolve } from "path";
import { defineConfig } from "vite";
import manifestSRI from 'vite-plugin-manifest-sri';
import { VitePWA } from "vite-plugin-pwa";

export default defineConfig({
  base: "/assets/dist/",
  css: {
    transformer: "lightningcss",

  },
  build: {
    outDir: resolve(__dirname, "../static/dist"),
    manifest: true,
    minify: true,
    target: "es2022",
    sourcemap: false,
    cssCodeSplit: true,
    reportCompressedSize: true,
    rollupOptions: {
      input: Object.fromEntries(
        fg.sync(["src/*.ts", "src/js/**/*.ts"]).map((file) => {
          const name = file.replace(/^src\//, "").replace(/\.ts$/, "");
          return [name, resolve(__dirname, file)];
        })
      ),
    },
  },
  plugins: [
    tailwindcss(),
    VitePWA({
      // 🔧 Generate service worker (offline caching, etc.)
      strategies: "generateSW", // Reliable default: Precaches assets
      registerType: "autoUpdate", // Auto-updates SW on changes
      // 📄 No auto-injection (no HTML entry); we'll manual register
      injectRegister: null,
      // 🖼️ Web App Manifest (install prompt, etc.)
      outDir: resolve(__dirname, "../static"),
      filename: 'sw.js',
      manifest: {
        name: "GoApp", // Customize
        short_name: "Goapp",
        description: "Your app description",
        theme_color: "#01C0F5", // Matches your CSS
        background_color: "#183746",
        display: "standalone",
        orientation: "portrait",
        start_url: "/", // Go root
        scope: "/", // SW controls entire site
        icons: [
          { src: "/assets/dist/favicon-16x16.png", sizes: "16x16", type: "image/png" },
          { src: "/assets/dist/favicon-32x32.png", sizes: "32x32", type: "image/png" },
          { src: "/assets/dist/favicon.ico", sizes: "64x64", type: "image/x-icon" },
          { src: "/assets/dist/pwa-64x64.png", sizes: "64x64", type: "image/png" },
          { src: "/assets/dist/pwa-192x192.png", sizes: "192x192", type: "image/png" },
          { src: "/assets/dist/pwa-512x512.png", sizes: "512x512", type: "image/png" },
          { src: "/assets/dist/maskable-icon-192x192.png", sizes: "192x192", type: "image/png", purpose: "maskable" },
          { src: "/assets/dist/maskable-icon-512x512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
          { src: "/assets/dist/apple-touch-icon-180x180.png", sizes: "180x180", type: "image/png" },
          { src: "/assets/dist/icon-optimized.svg", sizes: "any", type: "image/svg+xml", purpose: "any" }
        ]
      },
      devOptions: {
        enabled: false,
        type: "module",
      },
      // 📁 Output files
      workbox: {
        globPatterns: ["**/*.{js,css,html,png,ico,svg}"], // ✅ Cache icons
        navigateFallback: null,
      },
    }),
    manifestSRI({
      algorithms: ['sha384'],  // Recommended: Stronger than sha256, browser-supported
    }),
  ],

});
