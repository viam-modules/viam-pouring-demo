import adapter from "@sveltejs/adapter-auto";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

/** @type {import('@sveltejs/kit').Config} */
const config = {
  // Consult https://svelte.dev/docs/kit/integrations
  // for more information about preprocessors
  preprocess: vitePreprocess(),

  kit: {
    adapter: adapter(),
    typescript: {
      config: (tsconfig) => ({
        ...tsconfig,
        include: [
          // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment
          ...tsconfig.include,
          "../postcss.config.js",
          "../svelte.config.js",
          "../tailwind.config.ts",
        ],
      }),
    },
  },
};

export default config;
