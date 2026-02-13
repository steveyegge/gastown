// @ts-check

/**
 * Sidebar configuration for the project's docs folder (../docs)
 * @type {import('@docusaurus/plugin-content-docs').SidebarsConfig}
 */
const sidebars = {
  docsSidebar: [
    'overview',
    'INSTALLING',
    'glossary',
    'reference',
    'why-these-features',
    'formula-resolution',
    'beads-native-messaging',
    'mol-mall-design',
    {
      type: 'category',
      label: 'Concepts',
      items: [
        'concepts/convoy',
        'concepts/identity',
        'concepts/molecules',
        'concepts/polecat-lifecycle',
        'concepts/propulsion-principle',
      ],
    },
    {
      type: 'category',
      label: 'Design',
      items: [
        'design/architecture',
        'design/convoy-lifecycle',
        'design/dog-pool-architecture',
        'design/escalation',
        'design/escalation-system',
        'design/federation',
        'design/mail-protocol',
        'design/operational-state',
        'design/plugin-system',
        'design/property-layers',
        'design/watchdog-chain',
      ],
    },
  ],
};

export default sidebars;
