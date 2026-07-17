import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import glsl from 'vite-plugin-glsl'

// Dep pre-bundling runs without app plugins, so the .glsl/.hdr imports inside
// @viamrobotics/motion-tools dist need their own loaders there: shaders as
// text, hdr environment maps as dev-server URLs.
const glslAsText = {
  name: 'glsl-as-text',
  transform(code: string, id: string) {
    if (!/\.(glsl|vert|frag|vs|fs)$/.test(id)) return
    return { code: `export default ${JSON.stringify(code)};`, map: null }
  },
}

const hdrAsUrl = {
  name: 'hdr-as-url',
  load(id: string) {
    if (!id.endsWith('.hdr')) return
    const url = '/' + id.slice(id.indexOf('node_modules/'))
    return { code: `export default ${JSON.stringify(url)};`, map: null }
  },
}

// https://vite.dev/config/
export default defineConfig({
  base: "./",
  // hdr/glsl/define are required by @viamrobotics/motion-tools dist,
  // mirroring visualization/vite.config.ts.
  assetsInclude: ['**/*.hdr'],
  plugins: [glsl(), svelte(), tailwindcss()],
  optimizeDeps: {
    rolldownOptions: {
      plugins: [glslAsText, hdrAsUrl],
    },
  },
  define: {
    BACKEND_IP: JSON.stringify('localhost'),
    WS_PORT: JSON.stringify('3000'),
  },
})
