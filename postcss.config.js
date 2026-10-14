/** Configures the Tailwind and Autoprefixer PostCSS plugins. */
import tailwindConfig from './tailwind.config.js';

export default {
    plugins: {
        tailwindcss: tailwindConfig,
        autoprefixer: {},
    },
}
