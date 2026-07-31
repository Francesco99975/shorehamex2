import * as config from '@vite-pwa/assets-generator/config';

export default config.defineConfig({
  preset: {
    transparent: {
      sizes: [64, 192, 512],
      favicons: [[64, 'favicon.ico'], [32, 'favicon-32x32.png'], [16, 'favicon-16x16.png']],
      padding: 0
    },
    maskable: {
      sizes: [192, 512],
      padding: 0.2
    },
    apple: {
      sizes: [180],
      padding: 0
    }
  },
  images: ['public/icon-optimized.svg']
});
