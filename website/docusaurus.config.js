// @ts-check
// `@type` JSDoc annotations allow editor autocompletion and type checking
// (when paired with `@ts-check`).
// There are various equivalent ways to declare your Docusaurus config.
// See: https://docusaurus.io/docs/api/docusaurus-config

import { themes as prismThemes } from "prism-react-renderer";

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: "Gas Town",
  tagline:
    "Multi-agent orchestration system for coding agents with persistent work tracking.",
  favicon: "img/favicon.ico",

  // Future flags, see https://docusaurus.io/docs/api/docusaurus-config#future
  future: {
    v4: true, // Improve compatibility with the upcoming Docusaurus v4
  },

  // Set the production url of your site here
  url: "https://gastown.dev",
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: "/",

  // GitHub pages deployment config.
  organizationName: "steveyegge",
  projectName: "gastown",

  onBrokenLinks: "throw",

  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  markdown: {
    mermaid: true,
  },
  themes: ["@docusaurus/theme-mermaid"],

  presets: [
    [
      "classic",
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          // Reference the project's docs folder dynamically
          path: "../docs",
          sidebarPath: "./sidebars.js",
          editUrl: "https://github.com/steveyegge/gastown/tree/main/",
        },
        blog: false,
        theme: {
          customCss: "./src/css/custom.css",
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      image: "img/gastown-social-card.jpg",
      colorMode: {
        respectPrefersColorScheme: true,
      },
      navbar: {
        title: "Gas Town",
        logo: {
          alt: "Gas Town Logo",
          src: "img/logo.svg",
        },
        items: [
          {
            type: "docSidebar",
            sidebarId: "docsSidebar",
            position: "left",
            label: "Docs",
          },
          {
            href: "https://github.com/steveyegge/gastown",
            label: "GitHub",
            position: "right",
          },
        ],
      },
      footer: {
        style: "dark",
        links: [
          {
            title: "Docs",
            items: [
              {
                label: "Overview",
                to: "/docs/overview",
              },
              {
                label: "Installation",
                to: "/docs/INSTALLING",
              },
              {
                label: "Reference",
                to: "/docs/reference",
              },
            ],
          },
          {
            title: "Community",
            items: [
              {
                label: "Issues",
                href: "https://github.com/steveyegge/gastown/issues",
              },
              {
                label: "Beads",
                href: "https://github.com/steveyegge/beads",
              },
            ],
          },
          {
            title: "More",
            items: [
              {
                label: "GitHub",
                href: "https://github.com/steveyegge/gastown",
              },
              {
                label: "Steve Yegge on Medium",
                href: "https://steve-yegge.medium.com/",
              },
              {
                label: "Claude Code",
                href: "https://claude.ai/code",
              },
              {
                label: "Codex",
                href: "https://chatgpt.com/codex",
              },
              {
                label: "Gemini CLI",
                href: "https://github.com/google-gemini/gemini-cli",
              },
              {
                label: "Cursor Agent",
                href: "https://www.cursor.com/",
              },
              {
                label: "Auggie",
                href: "https://www.augmentcode.com/",
              },
              {
                label: "Amp",
                href: "https://ampcode.com/",
              },
            ],
          },
        ],
        copyright: `Copyright © ${new Date().getFullYear()} Gas Town. Built with Docusaurus.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
      },
    }),
};

export default config;
