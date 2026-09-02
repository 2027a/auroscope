> **SUPERSEDED BY ADR-0015 — DO NOT EXECUTE THIS PROMPT. HISTORICAL EVIDENCE ONLY.**

# Phase de design AURoscope

Travaille dans `/home/mathieu/.hermes/profiles/hephaistos/projects/auroscope` sur le dépôt `2027a/auroscope`.

Lis entièrement `AGENTS.md`, `README.md`, `docs/specification.md`, `docs/design-phase.md` et les ADR existants. Nous sommes en **phase de design**, pas en implémentation de production.

Revalide le comportement exact des versions pertinentes de Paru, Pacman et makepkg depuis leurs sources, manuels et tests; réalise des spikes jetables lorsque nécessaire. Prépare des propositions concrètes, avec options, compromis, recommandation, risques et preuves, pour : contrat Paru/Pacman, architecture Go et dépendances, modèle SQLite, protocole d’approbation anti-TOCTOU, contrats scanner/LLM, configuration XDG et nettoyage, modèle de menace et stratégie de tests.

Consulte `2027a/paru-llm-audit` uniquement comme référence historique ciblée. Ne porte pas son architecture ni son code par défaut. Soumets-moi les décisions structurantes avant de les déclarer acceptées; consigne ensuite les décisions approuvées en ADR. Ne rédige le plan d’implémentation qu’après validation du design et ne commence pas le code de production sans mon accord explicite.
