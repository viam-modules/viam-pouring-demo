import type { Config } from "tailwindcss";
import { theme } from '@viamrobotics/prime-core/theme';

export default {
  content: [
    "./src/**/*.{html,js,svelte,ts}",
    "./index.html",
  ],
  theme,
} satisfies Config;
