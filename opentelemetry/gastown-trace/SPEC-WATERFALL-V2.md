# Spec: Gastown Waterfall v2 — Chrome DevTools-style Agent Orchestration View

> **Référence autoritaire** : `/Users/pa/dev/third-party/gastown/docs/waterfall-spec.md`
> Ce document étend la référence avec les contraintes d'implémentation Go/frontend de `gastown-trace`.

---

## Context

Gastown est un système d'orchestration multi-agents tournant des agents Claude Code dans des sessions tmux. Les agents ont des rôles (mayor, deacon, witness, refinery, polecat, dog, boot, crew) et sont organisés en **rigs** (ex. `fai`, `mol`, `gt-wyvern`). Ils communiquent via **beads** (items de travail gérés par `bd`), **mails**, **slings** (dispatches de beads), et **prompts** envoyés dans des panes tmux.

Toute la télémétrie est dans VictoriaLogs (logs OTLP structurés) interrogée via LogsQL. L'outil Go `gastown-trace` existant requête VictoriaLogs et rend des vues HTML. La page `/waterfall` actuelle est un prototype — cette spec la remplace entièrement.

### Clé primaire : `run.id`

Chaque spawn d'agent génère un UUID unique — le **`run.id`** (`GT_RUN`) — propagé dans l'environnement tmux et dans `OTEL_RESOURCE_ATTRIBUTES` pour tous les sous-processus `bd`. C'est la clé de corrélation universelle sur tous les événements d'un run. **Toute logique de corrélation doit préférer `run.id` aux anciens champs `_stream`.**

---

## Goal

Construire une vue style **Chrome DevTools Network Waterfall** sur `/waterfall` qui montre la timeline complète d'une instance Gastown : chaque session agent, chaque échange inter-agents, chaque appel API — disposés horizontalement sur un axe temps partagé, avec filtrage interactif, drill-down, et visualisation des flux de communication.

Penser : Azure DevOps pipeline view × Chrome Network tab — pour un swarm d'agents IA.

---

## Data Sources (événements VictoriaLogs)

Toutes les données viennent d'appels `vlQuery()`. Types d'événements disponibles :

| Événement | Champs clés | Ce qu'il représente |
|-----------|------------|---------------------|
| `agent.instantiate` | `run.id`, `instance`, `town_root`, `agent_type`, `role`, `agent_name`, `session_id`, `rig` | **Racine de chaque run** — émis une fois par spawn |
| `session.start` | `run.id`, `session_id`, `role`, `status` | Session agent démarrée dans tmux |
| `session.stop` | `run.id`, `session_id`, `role`, `status` | Session agent terminée |
| `prime` | `run.id`, `role`, `hook_mode`, `formula`, `status` | Injection contexte de démarrage (formule TOML rendue) |
| `bd.call` | `run.id`, `subcommand`, `args`, `stdout`, `stderr`, `duration_ms`, `status` | Opération CLI bd |
| `claude_code.api_request` | `session.id`, `model`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `cost_usd`, `duration_ms` | Appel API LLM *(source : instrumentation OTEL de claude-code, indépendante de gastown)* |
| `claude_code.tool_result` | `session.id`, `tool_name`, `tool_parameters`, `duration_ms`, `success` | Exécution d'outil *(source : idem)* |
| `agent.event` | `run.id`, `session`, `native_session_id`, `agent_type`, `event_type`, `role` *(LLM role : `"assistant"` / `"user"`*, ≠ rôle Gastown)`, `content` | Tour de conversation agent (texte/tool_use/tool_result/thinking) |
| `prompt.send` | `run.id`, `session`, `keys_len`, `debounce_ms`, `status` | Prompt injecté dans l'agent via tmux *(le texte complet `keys` est à ajouter — P1)* |
| `pane.output` | `run.id`, `session`, `content` | Sortie brute tmux *(opt-in : `GT_LOG_PANE_OUTPUT=true`)* |
| `sling` | `run.id`, `bead`, `target`, `status` | Bead dispatché d'un agent à un autre |
| `mail` | `run.id`, `operation`, `msg.id`, `msg.from`, `msg.to`, `msg.subject`, `msg.body`, `msg.thread_id`, `msg.priority`, `msg.type`, `status` | Opération mail inter-agents |
| `nudge` | `run.id`, `target`, `status` | Agent relancé (nudge) |
| `polecat.spawn` | `run.id`, `name`, `status` | Sous-agent polecat spawné |
| `polecat.remove` | `run.id`, `name`, `status` | Polecat retiré |
| `done` | `run.id`, `exit_type` (COMPLETED/ESCALATED/DEFERRED), `status` | Agent a terminé son item de travail |
| `formula.instantiate` | `run.id`, `formula_name`, `bead_id`, `status` | Template de travail instancié |
| `convoy.create` | `run.id`, `bead_id`, `status` | Auto-convoy (batch) créé |
| `daemon.restart` | `run.id`, `agent_type` | Daemon redémarré |

> ⚠️ **Incohérence résolue — `mail`** : la V1 de ce spec ne listait que `operation` et `status`. Le schéma complet ci-dessus est celui de la référence (`waterfall-spec.md §1.3`). Utiliser `RecordMailMessage` pour les opérations avec contenu, `RecordMail` pour les opérations sans (list, archive-by-id).

> ⚠️ **Incohérence résolue — `agent.event.role`** : ce champ désigne le **rôle LLM** (`"assistant"` ou `"user"`), pas le rôle Gastown (mayor/witness/…). Le rôle Gastown est dans `agent.instantiate.role` et propagé via `gt.role` dans les `_stream` fields.

> ⚠️ **Incohérence résolue — `session.start`** : la V1 listait `gt.topic`, `gt.prompt`, `gt.agent` sur cet événement. Ces champs ne sont pas dans la référence. Ils proviennent d'une version antérieure des `_stream` fields. Les ignorer pour la logique de corrélation — préférer `agent.instantiate`.

### Attributs de ressource sur tous les événements

Deux systèmes coexistent — préférer les **attributs directs** (nouveau modèle) aux **`_stream` fields** (legacy) :

**Attributs directs (nouveau modèle, autoritaire) :**
- `run.id` — UUID run (clé primaire)
- `instance` — `hostname:basename(town_root)` (ex. `laptop:gt`)
- `role` — rôle Gastown (mayor, witness, polecat, …)
- `rig` — nom du rig (vide = town-level)
- `session_id` — nom de la pane tmux

**`_stream` fields (legacy, utiles pour les anciens events) :**
- `gt.role`, `gt.rig`, `gt.session`, `gt.actor`, `gt.agent`, `gt.town`

---

## Layout

### Deux niveaux de vue

**Niveau 1 : Vue instance** (`/waterfall`) — Swim lanes de tous les runs actifs/récents, groupés par rig, sur un axe temps partagé.

**Niveau 2 : Vue run detail** (`/waterfall?run=<uuid>` ou panneau de détail au clic) — Timeline hiérarchique d'un run individuel, depuis `agent.instantiate` jusqu'à `session.stop`.

### Structure globale

```
┌─────────────────────────────────────────────────────────────────────┐
│ nav: [Dashboard] [Flow] [Waterfall*] [Sessions] [Beads] ...       │
│      window: [1h] [24h] [7d] [30d] [custom range]                 │
├─────────────────────────────────────────────────────────────────────┤
│ INSTANCE: laptop:gt   town: /Users/pa/gt                           │
│ FILTERS BAR                                                        │
│ [Rig ▼] [Role ▼] [Agent ▼] [Event types ▼] [Search ___________]  │
├─────────────────────────────────────────────────────────────────────┤
│ SUMMARY CARDS                                                      │
│ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐           │
│ │ 12   │ │ 3    │ │ 847  │ │ 42   │ │$1.23 │ │ 2m30s│           │
│ │ Runs │ │ Rigs │ │Events│ │Beads │ │ Cost │ │ Span │           │
│ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘           │
├──────────────┬──────────────────────────────────────────────────────┤
│ SWIM LANES   │  TIME AXIS ──────────────────────────────────►      │
│              │  0s    30s    1m     1m30   2m     2m30    3m       │
│──────────────┼──────────────────────────────────────────────────────│
│              │                                                      │
│ ── fai ──    │  (rig header, collapsible)                          │
│              │                                                      │
│ fai/mayor    │  ████████████████████████████████████████████████── │
│   API calls  │    ▪▪  ▪▪▪  ▪▪    ▪▪▪▪▪   ▪▪  ▪▪▪  ▪▪           │
│   tools      │     ◆  ◆◆    ◆      ◆◆◆    ◆    ◆               │
│              │        ╔══▶ sling:bead-42 ══════════▶               │
│ fai/deacon   │         ░░░░░░░░░░░░░░░░░░░░░░░░────              │
│   API calls  │           ▪▪  ▪▪▪   ▪▪▪  ▪▪▪                      │
│              │              ╔══▶ mail → fai/witness ══▶            │
│ fai/witness  │               ░░░░░░░░░░░░░░░░░░░──                │
│   API calls  │                 ▪▪  ▪▪  ▪▪  ▪▪                     │
│              │                                                      │
│ ── mol ──    │  (rig header, collapsible)                          │
│              │                                                      │
│ mol/witness  │       ██████████████████████████████████████████──── │
│ mol/polecat  │              ░░░░░░░░░░░░░░──                       │
│   ↑jana      │              ░░░░░░░░░──                            │
│              │                                                      │
├──────────────┴──────────────────────────────────────────────────────┤
│ DETAIL PANEL (clic sur n'importe quel élément)                     │
│ ┌───────────────────────────────────────────────────────────────┐  │
│ │ Run: 6ba7b810…  Role: witness  Rig: fai  Agent: witness      │  │
│ │ Started: 14:32:05  Duration: 1m42s  Cost: $0.3241            │  │
│ │                                                               │  │
│ │ [14:32:01] ● instantiate   claudecode/fai-witness             │  │
│ │ [14:32:05] ─ session.start                                   │  │
│ │ [14:32:06]   prime         polecat formula (2 KB)            │  │
│ │ [14:32:06] ▶ prompt.send   "You have bead gt-abc…"          │  │
│ │ [14:32:08] ◀ thinking      847 chars                         │  │
│ │ [14:32:10] ◀ text          "I'll review the assigned bead…"  │  │
│ │ [14:32:11] 🔧 tool_use     bd list --assignee=fai/witness    │  │
│ │ [14:32:11]   bd.call       list (38ms) ✓                     │  │
│ │ [14:32:11] ↩ tool_result   [{id:"bead-42"…}]                │  │
│ │ [14:32:15] 🔧 tool_use     Bash "git diff HEAD~1"            │  │
│ │ [14:32:18] ↩ tool_result   (320 lines)                       │  │
│ │ [14:32:25] ◀ text          "The changes look correct…"       │  │
│ │ [14:32:26] 🔧 tool_use     bd update bead-42 --status=done   │  │
│ │ [14:32:26] ■ done          COMPLETED                         │  │
│ │ [14:32:26] ─ session.stop                                    │  │
│ └───────────────────────────────────────────────────────────────┘  │
├─────────────────────────────────────────────────────────────────────┤
│ COMMUNICATION MAP (section collapsible)                            │
│                                                                     │
│   mayor ──sling──▶ deacon ──mail──▶ witness                       │
│     │                                    │                         │
│     └──────────── mail ◀─────────────────┘                         │
│                                                                     │
│   mayor ──spawn──▶ polecat/jana                                    │
│     │               │                                              │
│     └── nudge ──────┘                                              │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Swim lanes — détail

Chaque **run agent** (`agent.instantiate`) obtient une swim lane horizontale. Les lanes sont groupées par **rig**, avec des headers de rig collapsibles. Dans chaque lane :

1. **Barre de session** : barre colorée pleine largeur (couleur = rôle) de `session.start` à `session.stop` (ou maintenant si toujours en cours). Animation pulsante si en cours.

2. **Ticks API** : petites marques verticales sur la barre de session pour chaque `claude_code.api_request`. Intensité de couleur = coût. Hover : modèle, tokens, coût, durée.

3. **Markers outils** : marqueurs diamant sous la barre pour chaque `claude_code.tool_result`. Couleur = succès (vert) / échec (rouge). Hover : nom de l'outil, commande, durée.

4. **Flèches inter-agents** : flèches horizontales entre lanes montrant les communications :
   - **Sling** (dispatch bead) : flèche pleine, labelisée avec le bead ID
   - **Mail** (send/deliver) : flèche ondulée, labelisée avec `msg.subject` ou `msg.from→msg.to`
   - **Nudge** : flèche pointillée
   - **Polecat spawn** : flèche épaisse vers la lane enfant
   - **Done/escalate** : flèche retour vers le parent

   > ⚠️ **Suggestion** : La V1 définissait un type `assign` (dérivé de `bd update --assignee`). Ce n'est pas un événement natif — c'est une heuristique. L'afficher comme `bd.call` avec `subcommand=update` et args contenant `--assignee`, pas comme un type de communication à part entière.

5. **Overlay lifecycle bead** : segments colorés optionnels sur les barres de session montrant quel bead est en cours de travail (depuis la corrélation des `bd.call` create/update).

### Axe temps

- Axe temps horizontal partagé en haut, auto-scaling sur la fenêtre
- Marques à intervalles sensibles (toutes les 10s, 30s, 1m, 5m, etc.)
- Lignes de grille verticales (subtiles) pour l'alignement
- Zoom : molette souris sur la zone timeline
- Pan : clic-drag sur la zone timeline
- Marqueur temps courant (si vue live/récente) : ligne verticale rouge

### Filtres

| Filtre | Type | Source |
|--------|------|--------|
| Rig | multi-select dropdown | `rig` attribut sur `agent.instantiate` |
| Role | multi-select dropdown | `role` : mayor, deacon, witness, refinery, polecat, dog, boot, crew |
| Agent | multi-select dropdown | `agent_name` ou `session_id` |
| Event types | checkbox group | Runs, API calls, Tool calls, BD calls, Slings, Mails, Nudges, Spawns |
| Search | text input | Recherche plein-texte sur contenu, bead IDs, noms d'outils |

Les filtres sont URL query-param driven (`?rig=fai&role=witness&types=api,tool`) pour le partage de liens.

### Panneau de détail — Panel droit (style Chrome DevTools Network)

Cliquer sur une ligne du waterfall ouvre un **panneau latéral droit** qui s'affiche à côté du waterfall (layout split vertical, ~40% de la largeur), exactement comme le panneau de détail de Chrome DevTools Network. Le waterfall se redimensionne pour céder la place — il ne disparaît pas.

```
┌──────────────────────────┬──────────────────────────────────────┐
│  WATERFALL (60%)         │  DETAIL PANEL (40%)                  │
│                          │                                       │
│  fai/mayor  ████████─    │  ┌─ fai-witness / polecat ──────[✕]─┐│
│  fai/deacon  ░░░░░──     │  │  run: 6ba7b810…  dur: 4m32s      ││
│► fai/witness ░░░────     │  │  rig: wyvern  cost: $0.0341       ││
│  mol/witness ████──      │  ├──────────────────────────────────┤│
│                          │  │ [Overview][Prompt][Conversation]  ││
│                          │  │ [BD Calls][Mails][Timeline]       ││
│                          │  ├──────────────────────────────────┤│
│                          │  │                                   ││
│                          │  │  (contenu de l'onglet actif)      ││
│                          │  │                                   ││
│                          │  └──────────────────────────────────┘│
└──────────────────────────┴──────────────────────────────────────┘
```

#### Onglets du panneau (selon le type d'élément cliqué)

**Clic sur une lane de run** → panneau run avec 6 onglets :

| Onglet | Contenu |
|--------|---------|
| **Overview** | Métadonnées : `run.id`, `role`, `rig`, `agent_name`, `agent_type`, `session_id`, `instance`, `started_at`, `ended_at`, durée, coût total, nombre d'events |
| **Prompt** | Texte complet du ou des `prompt.send` reçus par l'agent (attribut `keys` si disponible, sinon `keys_len` + mention manquante). Police monospace, fond sombre, scrollable. Si le prompt contient du Markdown, le rendre. |
| **Conversation** | Tous les `agent.event` du run, affichés en bulles de chat : `thinking` (lavande, italique), `assistant/text` (vert foncé, aligné droite), `user/text` (vert clair, aligné gauche), `tool_use` (ambre, bloc code), `tool_result` (bleu, bloc code). Contenu intégral, pas de troncature. Scrollable. |
| **BD Calls** | Table de tous les `bd.call` : `time`, `subcommand`, `args`, `duration_ms`, `status`. Si `GT_LOG_BD_OUTPUT=true`, afficher `stdout` dans un `<details>` collapsible. |
| **Mails** | Table de tous les `mail` events : `operation`, `msg.from`, `msg.to`, `msg.subject`, `msg.priority`. Corps complet (`msg.body`) dans un `<details>` collapsible. |
| **Timeline** | Mini-waterfall du run uniquement : même vue horizontale que le waterfall global mais zoomée sur ce run seul, avec les sous-events imbriqués (voir §Nesting). |

**Clic sur un tick API** → panneau API avec 2 onglets :

| Onglet | Contenu |
|--------|---------|
| **Headers** | Modèle, `session.id`, timestamps, durée |
| **Tokens** | Table : input / output / cache_read tokens, coût USD. Barre visuelle proportionnelle. |

**Clic sur un marker outil** → panneau Tool avec 2 onglets :

| Onglet | Contenu |
|--------|---------|
| **Summary** | Nom de l'outil, durée, succès/échec, `session.id` |
| **Parameters** | `tool_parameters` JSON formatté avec coloration syntaxique (JSON.stringify indent 2). |

**Clic sur une flèche communication** → panneau Comm avec 2 onglets :

| Onglet | Contenu |
|--------|---------|
| **Info** | Type (`sling`/`mail`/`nudge`/`spawn`/`done`), source, cible, timestamp, bead ID si applicable |
| **Bead** | Pour `sling` : lifecycle complet du bead depuis `/bead/{id}` (table des transitions). Pour `mail` : corps complet `msg.body`. |

#### Comportement du panneau

- **Ouverture** : slide-in depuis la droite, animation 150ms
- **Fermeture** : bouton `✕` en haut à droite, ou touche `Escape`
- **Redimensionnement** : drag sur le bord gauche du panneau (largeur entre 25% et 70%)
- **Persistance de l'onglet actif** : mémorisé par type (run/api/tool/comm) pendant la session
- **Navigation entre runs** : touches `↑` / `↓` pour passer au run précédent/suivant dans la liste sans fermer le panneau
- **Lien externe** : bouton "Open in full view" → `/session/{session_id}` ou `/bead/{id}`

### Communication map

Section collapsible sous le waterfall montrant un **node-link diagram** de toute la communication inter-agents dans la fenêtre :

- Nœuds = agents (colorés par rôle)
- Arêtes = événements de communication (slings, mails, spawns, nudges, dones)
- Épaisseur de l'arête = fréquence
- Label de l'arête = count + dernier bead ID ou subject mail
- Survol d'un nœud : highlight de toutes ses arêtes de communication
- Clic sur un nœud : filtre le waterfall sur cet agent

### Codes couleur

| Événement | Couleur |
|-----------|---------|
| `agent.instantiate` | violet |
| `session.start` / `session.stop` | gris |
| `prime` / `prime.context` | bleu |
| `prompt.send` | cyan |
| `agent.event` thinking | lavande |
| `agent.event` text assistant | vert foncé |
| `agent.event` tool_use | orange |
| `agent.event` tool_result | orange clair |
| `agent.event` user | vert |
| `bd.call` | rouge |
| `mail` | jaune |
| `sling` / `nudge` | rose |
| `done` COMPLETED | vert vif |
| `done` ESCALATED / DEFERRED | orange vif |
| statut `"error"` | bordure rouge vif |

### Règles de nesting (vue run detail)

Les logs OTel ne portant pas de parent span ID natif, la hiérarchie est reconstruite par :
1. Groupement sur `run.id`
2. Ordonnancement chronologique par `_time`
3. Règles suivantes :

```
agent.instantiate                    ← racine absolue (1 par run)
  ├─ session.start                   ← cycle de vie tmux
  ├─ prime                           ← injection contexte
  ├─ prompt.send                     ← daemon → agent
  │
  ├─ agent.event [user/text]         ← message texte reçu
  ├─ agent.event [user/tool_result]  ← résultat d'outil reçu
  │
  ├─ agent.event [assistant/thinking]
  ├─ agent.event [assistant/text]
  ├─ agent.event [assistant/tool_use]  ← appel outil
  │    ↳ bd.call                         si tool = bd (fenêtre temporelle)
  │    ↳ mail                            si tool = mail
  │    ↳ sling                           si tool = gt sling
  │    ↳ nudge                           si tool = gt nudge
  │
  ├─ done                            ← fin de travail
  └─ session.stop                    ← fin lifecycle
```

Tout événement sans parent inférable → affiché à plat.

---

## Implementation notes

### Code existant à réutiliser

- `data.go` : `loadSessions()`, `loadBeadLifecycles()`, `loadAPIRequests()`, `loadToolCalls()`, `loadBDCalls()`, `loadFlowEvents()`, `loadPaneOutput()`, `correlateClaudeSessions()` — structs typés utilisables
- `vl.go` : `vlQuery()` pour les requêtes VictoriaLogs, `extractStreamField()` pour parser les `_stream` attributes
- `main.go` : pattern handler existant, template helpers (`roleColor`, `fmtTime`, `fmtDur`, `fmtCost`, etc.)
- `waterfall.go` : `rigFromSession()`, `loadWaterfallData()` — partiellement réutilisable, refactoring profond nécessaire

### Nouvelles données à charger

1. **Runs** : `vlQuery(cfg.LogsURL, "agent.instantiate", limit, since, end)` — champs : `run.id`, `instance`, `town_root`, `agent_type`, `role`, `agent_name`, `session_id`, `rig`
2. **Slings** : `vlQuery(cfg.LogsURL, "sling", limit, since, end)` — champs : `run.id`, `bead`, `target`, `status`
3. **Mails** : `vlQuery(cfg.LogsURL, "mail", limit, since, end)` — champs : `run.id`, `operation`, `msg.from`, `msg.to`, `msg.subject`, `msg.body`, `msg.thread_id`, `msg.priority`, `msg.type`, `status`
4. **Nudges** : `vlQuery(cfg.LogsURL, "nudge", limit, since, end)` — champs : `run.id`, `target`, `status`
5. **Spawns** : `vlQuery(cfg.LogsURL, "polecat.spawn", limit, since, end)` — champs : `run.id`, `name`, `status`
6. **Dones** : `vlQuery(cfg.LogsURL, "done", limit, since, end)` — champs : `run.id`, `exit_type`, `status`
7. **Prime** : `vlQuery(cfg.LogsURL, "prime", limit, since, end)` — champs : `run.id`, `role`, `formula`, `hook_mode`, `status`

> ⚠️ **Suggestion** : Requêter d'abord les `agent.instantiate` pour obtenir tous les `run.id` de la fenêtre, puis requêter tous les events avec `run.id:<uuid1> OR run.id:<uuid2> OR …` pour éviter N+1 requêtes. Voir `waterfall-spec.md §4.1`.

### Pipeline de données

```
loadWaterfallV2Data(cfg, since, filters) →
  1. Load agent.instantiate  → liste des runs → group by rig
  2. Load session.start/stop → durées des runs
  3. Load prime              → contexte de démarrage par run
  4. Load API requests       → assign to runs via correlateClaudeSessions() + run.id
  5. Load tool calls         → assign to runs via session.id
  6. Load agent events       → assign to runs via native_session_id + run.id
  7. Load BD calls           → extraire slings, assigns, creates
  8. Load slings/mails       → construire les arêtes de communication (source run → target)
  9. Load spawns/dones       → construire les arêtes de lifecycle
  10. Compute time axis      → min(started_at) to max(ended_at or now)
  11. Apply filters          → rig, role, agent, event type
  12. Serialize to JSON      → send to frontend for rendering
```

### Requêtes VictoriaLogs

```
# Tous les runs récents (vue instance)
GET /select/logsql/query?query=_msg:agent.instantiate AND instance:laptop:gt AND _time:[now-1h,now]&limit=100

# Tous les events d'un run
GET /select/logsql/query?query=run.id:<uuid>&limit=10000

# Filtrer par rig
GET /select/logsql/query?query=_msg:agent.instantiate AND rig:fai

# Filtrer par rôle
GET /select/logsql/query?query=_msg:agent.instantiate AND role:polecat
```

### Frontend rendering

Le waterfall DOIT être rendu côté client (JavaScript + Canvas ou SVG) pour l'interactivité (zoom, pan, hover, clic). Le handler Go sert :

1. Une page HTML avec le shell (nav, filtres, summary cards, panneau de détail)
2. Un bloc `<script>` avec les données waterfall en JSON : `const DATA = {{.JSONData}};`
3. Le JavaScript qui rend le waterfall dans un container `<canvas>` ou SVG

Utiliser Canvas pour la performance (centaines d'events). SVG convient pour la communication map.

### API endpoint

Ajouter `GET /api/waterfall.json?window=24h&rig=fai&role=witness` qui retourne les données structurées en JSON. Cela permet :
- La page `/waterfall` de fetcher les données dynamiquement (changements de filtre sans rechargement complet)
- Un frontend séparé peut consommer la même API

### JSON shape

```typescript
interface WaterfallEvent {
  id:        string;       // ID interne VictoriaLogs
  run_id:    string;       // UUID run GASTOWN (GT_RUN)
  body:      string;       // nom d'événement ("bd.call", "agent.event", "mail", …)
  timestamp: string;       // RFC3339
  severity:  "info" | "error";
  attrs: {
    // Présents sur tous les événements
    instance?:          string;
    town_root?:         string;
    session_id?:        string;
    rig?:               string;
    role?:              string;   // rôle Gastown sur agent.instantiate/session.*
                                  // rôle LLM ("assistant"/"user") sur agent.event
    agent_type?:        string;
    agent_name?:        string;
    status?:            string;
    // agent.event
    event_type?:        string;
    "agent.role"?:      string;  // "assistant" | "user" (LLM role, alias de role sur agent.event)
    content?:           string;  // contenu intégral — aucune troncature
    native_session_id?: string;
    // bd.call
    subcommand?:        string;
    args?:              string;
    duration_ms?:       number;
    stdout?:            string;
    stderr?:            string;
    // mail
    "msg.id"?:          string;
    "msg.from"?:        string;
    "msg.to"?:          string;
    "msg.subject"?:     string;
    "msg.body"?:        string;  // corps complet — aucune troncature
    "msg.thread_id"?:   string;
    "msg.priority"?:    string;
    "msg.type"?:        string;
    // prime
    formula?:           string;
    hook_mode?:         boolean;
    // sling
    bead?:              string;
    target?:            string;
    // done
    exit_type?:         string;
    [key: string]:      unknown;
  };
}

interface WaterfallRun {
  run_id:      string;
  instance:    string;
  town_root:   string;
  agent_type:  string;
  role:        string;
  agent_name:  string;
  session_id:  string;
  rig:         string;
  started_at:  string;
  ended_at?:   string;      // présent si session.stop reçu
  duration_ms?: number;
  running:     boolean;
  cost?:       number;      // depuis claude_code.api_request
  events:      WaterfallEvent[];
}

interface WaterfallInstance {
  instance:   string;
  town_root:  string;
  window:     { start: string; end: string };
  summary: {
    runCount:      number;
    rigCount:      number;
    eventCount:    number;
    beadCount:     number;
    totalCost:     number;
    totalDuration: string;
  };
  rigs: Array<{
    name:      string;
    collapsed: boolean;
    runs:      WaterfallRun[];
  }>;
  communications: Array<{
    time:      string;
    type:      "sling" | "mail" | "nudge" | "spawn" | "done";
    from:      string;   // run_id ou actor (rig/role)
    to:        string;
    beadID?:   string;
    label:     string;
    // mail seulement
    subject?:  string;
    body?:     string;
  }>;
  beads: Array<{
    id:        string;
    title:     string;
    type:      string;
    state:     string;
    createdBy: string;
    assignee:  string;
    createdAt: string;
    doneAt?:   string;
  }>;
}
```

> ⚠️ **Incohérence résolue** : La V1 avait `rigs > lanes > apiCalls/toolCalls/agentEvents` (séparation artificielle). La nouvelle shape normalise tout comme `WaterfallEvent[]` dans chaque `WaterfallRun`, aligné avec la référence TypeScript. Les `apiCalls` et `toolCalls` issus de `claude_code.*` restent séparés dans `events` avec leur `body` spécifique.

> ⚠️ **Suggestion** : Le type `"assign"` dans `communications` (V1) est supprimé — il n'existe pas comme événement natif. L'assignation d'un bead à un agent via `bd update --assignee` est visible dans `bd.call` events, pas en tant que communication inter-agents.

---

## Variables d'environnement

| Variable | Où positionné | Rôle |
|----------|--------------|------|
| `GT_RUN` | env tmux session + subprocess | UUID run, clé waterfall |
| `GT_OTEL_LOGS_URL` | démarrage daemon | endpoint VictoriaLogs OTLP |
| `GT_OTEL_METRICS_URL` | démarrage daemon | endpoint VictoriaMetrics OTLP |
| `GT_LOG_AGENT_OUTPUT` | opérateur | opt-in streaming JSONL Claude |
| `GT_LOG_BD_OUTPUT` | opérateur | opt-in contenu bd stdout/stderr |
| `GT_LOG_PANE_OUTPUT` | opérateur | opt-in sortie brute pane tmux |

`GT_RUN` est surfacé en `gt.run_id` dans `OTEL_RESOURCE_ATTRIBUTES` pour tous les subprocessus `bd`, corrélant leur télémétrie au run parent.

---

## Interactions

| Action | Résultat |
|--------|----------|
| Hover sur barre de session | Tooltip léger : run.id (8 chars), role, rig, durée, coût |
| **Clic sur une lane (run)** | **Panneau droit slide-in : onglets Overview / Prompt / Conversation / BD Calls / Mails / Timeline** |
| Hover sur tick API | Tooltip : modèle, tokens, coût, latence |
| **Clic sur tick API** | **Panneau droit : onglets Headers / Tokens** |
| Hover sur marker outil | Tooltip : nom de l'outil, durée, succès |
| **Clic sur marker outil** | **Panneau droit : onglets Summary / Parameters (JSON formatté)** |
| Hover sur flèche communication | Highlight lanes source + cible, label comm |
| **Clic sur flèche communication** | **Panneau droit : onglets Info / Bead ou Info / Mail (corps complet)** |
| Molette sur timeline | Zoom in/out centré sur le curseur |
| Clic-drag sur timeline | Pan gauche/droite |
| Clic header rig | Collapse/expand groupe rig |
| Clic nœud dans comm map | Filtre le waterfall sur cet agent |
| Touche `Escape` | Ferme le panneau droit |
| Touches `↑` / `↓` (panneau ouvert) | Run précédent / suivant sans fermer le panneau |
| Drag bord gauche du panneau | Redimensionne la largeur (25%–70%) |
| Bouton "Open in full view" | Navigue vers `/session/{session_id}` ou `/bead/{id}` |

---

## Non-goals (v1)

- Real-time streaming (SSE/WebSocket) — utiliser `/live-view` pour ça
- État éditable (pas de mise à jour de beads depuis cette vue)
- Diff historique (comparer deux fenêtres temporelles)
- Layout mobile

---

## Acceptance criteria

1. `/waterfall` rend une timeline Canvas horizontale avec swim lanes groupées par rig
2. Chaque swim lane correspond à un `run.id` issu de `agent.instantiate`
3. Tous les filtres actifs se reflètent dans les URL query params et persistent au rechargement
4. **Clic sur une lane ouvre le panneau droit (split vertical) avec les 6 onglets**
5. **L'onglet Prompt affiche le texte complet du `prompt.send` (`keys`) en monospace, non tronqué**
6. **L'onglet Conversation affiche tous les `agent.event` en bulles de chat, contenu intégral**
7. **L'onglet BD Calls liste tous les `bd.call` avec `stdout` collapsible**
8. **L'onglet Mails liste tous les `mail` avec `msg.body` collapsible**
9. **Le panneau se redimensionne par drag sur son bord gauche**
10. **Touches `↑`/`↓` naviguent entre runs sans fermer le panneau**
11. Les flèches de communication inter-agents se rendent entre les bonnes swim lanes
12. Zoom/pan fonctionne fluidement pour jusqu'à 50 runs et 5000 events
13. `/api/waterfall.json` retourne les données structurées complètes
14. La section communication map rend un node-link diagram lisible
15. Thème sombre cohérent avec les pages gastown-trace existantes

---

## Statut d'implémentation (référence : waterfall-spec.md §7)

| Composant | Statut |
|-----------|--------|
| `run.id` généré au spawn (lifecycle, polecat, witness, refinery) | ✅ |
| `GT_RUN` propagé env tmux + subprocess `agent-log` | ✅ |
| `GT_RUN` dans `OTEL_RESOURCE_ATTRIBUTES` pour bd | ✅ |
| `run.id` injecté dans chaque événement OTel | ✅ |
| `agent.instantiate` avec `instance`, `role`, `town_root` | ✅ |
| `RecordMailMessage` avec contenu complet | ✅ (appels à ajouter dans `mail/`) |
| Contenu `agent.event` sans troncature | ✅ |
| Contenu bd stdout/stderr sans troncature | ✅ |
| Texte complet du prompt dans `prompt.send` (attribut `keys`) | ⬜ P1 |
| `RecordMailMessage` appelé depuis `mail/router` + `delivery` | ⬜ P2 |
| Bead ID du travail dans `agent.instantiate` | ⬜ P2 |
| Token usage depuis JSONL Claude | ⬜ P3 |
| **Panneau droit avec onglets (Overview/Prompt/Conversation/BD/Mails/Timeline)** | ⬜ à implémenter |
| Frontend waterfall v2 (base) | ✅ implémenté |
