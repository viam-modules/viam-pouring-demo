import type { Config } from 'tailwindcss'

import { plugins } from '@viamrobotics/prime-core/plugins'
import { theme } from '@viamrobotics/prime-core/theme'

// Only scanned for the embedded motion-tools viewer (PlannerDebug); the pour
// dashboard itself uses carbon, not tailwind.
export default {
  darkMode: 'class',
  content: [
    './node_modules/@viamrobotics/motion-tools/dist/**/*.{js,svelte}',
    './node_modules/@viamrobotics/prime-core/**/*.{ts,svelte}',
  ],
  theme,
  plugins,
} satisfies Config
