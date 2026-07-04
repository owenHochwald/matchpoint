import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        void: "#06080f",
        grid: "#122033",
        neon: "#39ff88",
        cyan: "#36d8ff",
        danger: "#ff3d71",
      },
      boxShadow: {
        terminal: "0 0 0 1px rgba(57,255,136,0.22), 0 18px 80px rgba(0,0,0,0.45)",
        glow: "0 0 24px rgba(57,255,136,0.22)",
      },
      fontFamily: {
        mono: ["JetBrains Mono", "SFMono-Regular", "Consolas", "monospace"],
        sans: ["Inter", "ui-sans-serif", "system-ui", "sans-serif"],
      },
    },
  },
  plugins: [],
} satisfies Config;
