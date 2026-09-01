import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        brand: {
          50: "#eef7ff",
          100: "#d9edff",
          200: "#bce0ff",
          300: "#8ecdff",
          400: "#59b0ff",
          500: "#3392ff",
          600: "#1b72f6",
          700: "#145be2",
          800: "#174bb7",
          900: "#194190",
        },
      },
    },
  },
  plugins: [],
};
export default config;