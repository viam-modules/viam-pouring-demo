import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import glsl from 'vite-plugin-glsl'

// https://vite.dev/config/
export default defineConfig({
  base: "./",
  // hdr/glsl/define are required by the linked @viamrobotics/motion-tools dist,
  // mirroring visualization/vite.config.ts.
  assetsInclude: ['**/*.hdr'],
  plugins: [glsl(), svelte(), tailwindcss()],
  define: {
    BACKEND_IP: JSON.stringify('localhost'),
    WS_PORT: JSON.stringify('3000'),
  },
  resolve: {
    // motion-tools is a file: symlink; without dedupe its imports resolve into
    // the visualization repo's own node_modules, yielding duplicate singletons
    // (svelte/three contexts) and uncompilable TS .svelte files.
    dedupe: [
      'svelte',
      'three',
      '@threlte/core',
      '@threlte/extras',
      '@threlte/rapier',
      '@threlte/xr',
      '@viamrobotics/sdk',
      '@viamrobotics/svelte-sdk',
      '@viamrobotics/prime-core',
      '@tanstack/svelte-query',
      'lucide-svelte',
    ],
  },
})
