import { defineConfig, markdown, openapi } from "sourcey";

export default defineConfig({
  name: "Edge Proxy",
  siteUrl:
    process.env.SOURCEY_SITE_URL || "https://ktrubilo9.github.io",
  baseUrl: process.env.SOURCEY_BASE_URL || "/edge-proxy",
  prettyUrls: "slash",
  theme: {
    preset: "api-first",
    colors: {
      primary: "#2563eb",
      light: "#60a5fa",
      dark: "#1d4ed8",
    },
  },
  codeSamples: ["curl", "go", "javascript", "python"],
  navigation: {
    tabs: [
      {
        tab: "Guide",
        slug: "",
        source: markdown({
          groups: [
            {
              group: "Admin API",
              pages: ["docs/api-reference"],
            },
          ],
        }),
      },
      {
        tab: "API Reference",
        slug: "api",
        source: openapi("./docs/openapi.yaml"),
      },
    ],
  },
  navbar: {
    links: [
      {
        type: "github",
        href: "https://github.com/ktrubilo9/edge-proxy",
      },
    ],
  },
  footer: {
    socials: {
      github: "https://github.com/ktrubilo9/edge-proxy",
    },
  },
});
