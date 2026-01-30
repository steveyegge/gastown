import clsx from 'clsx';
import Heading from '@theme/Heading';
import useBaseUrl from '@docusaurus/useBaseUrl';
import styles from './styles.module.css';

const FeatureList = [
  {
    title: 'Persistent work state',
    image: '/img/feature-persistent.jpg',
    description: (
      <>
        Hooks store agent output in git worktrees so work survives restarts and
        can be audited or rolled back.
      </>
    ),
  },
  {
    title: 'Mayor-led coordination',
    image: '/img/feature-mayor.jpg',
    description: (
      <>
        The Mayor creates convoys, assigns beads, and keeps large multi-agent
        efforts organized.
      </>
    ),
  },
  {
    title: 'Beads-based tracking',
    image: '/img/feature-beads.jpg',
    description: (
      <>
        Beads provide structured, git-backed issue tracking that integrates with
        convoys and automated workflows.
      </>
    ),
  },
];

function Feature({image, title, description}) {
  return (
    <div className={clsx('col col--4')}>
      <div className="text--center">
        <img className={styles.featureImg} src={useBaseUrl(image)} alt={title} />
      </div>
      <div className="text--center padding-horiz--md">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

export default function HomepageFeatures() {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}
