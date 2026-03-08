# Architecture Tooling Decision

## Decision

Adopt C4-style architecture documentation in the repo now, and keep the structure ready for later C4InterFlow modelling, but do not introduce the full C4InterFlow toolchain yet.

## Why This Decision

This repo needs immediate, maintainable architecture documentation more than it needs another build toolchain.

The current codebase is:
- a single Go service
- hexagonal in structure
- documented in Markdown already
- deployed through GitHub Actions, OpenTofu, Docker Compose, and Cloudflare Tunnel

That makes repo-local text-first architecture docs the highest-value step today.

## Research Summary

### C4InterFlow

`C4InterFlow` is an architecture-as-code framework with YAML/JSON/C# input, auto-generated views, flows, queries, and CLI-driven publishing. It is designed for version-controlled architecture repositories and supports CI/CD generation of diagrams and documentation. It also requires Java for the embedded PlantUML path and brings a broader modelling surface than this repo currently needs.

Source:
- https://github.com/SlavaVedernikov/C4InterFlow

### Official C4 guidance

The official C4 tooling guidance emphasizes long-lived documentation, Git-friendly formats, diffability, and the distinction between simple diagramming and reusable modelling. Those points fit this repo well.

Source:
- https://c4model.com/tooling

### freeCodeCamp C4 article

The freeCodeCamp article shows the practical value of text-based C4 modelling and CI automation for keeping diagrams close to code. That supports adopting a checked-in architecture model rather than separate slideware.

Source:
- https://www.freecodecamp.org/news/how-to-create-software-architecture-diagrams-using-the-c4-model/

### Ilograph and the HN discussion

The Ilograph article is useful as a critique of abstraction-first modelling: concrete resources matter, especially for existing systems. The HN discussion reinforces two good constraints for this repo:
- do not trap architecture docs in a SaaS
- do not over-abstract away the real deployment/runtime pieces

Sources:
- https://www.ilograph.com/blog/posts/concrete-diagramming-models/
- https://news.ycombinator.com/item?id=35302395

### Archi / ArchiMate

Archi is powerful, but it targets a heavier enterprise modelling style than this repo currently needs. It is useful as an alternative if the project grows into broader enterprise architecture, but it is not the right first move here.

Source:
- https://www.archimatetool.com/

## Resulting Approach

Phase 1, now:
- keep architecture docs in Markdown under `docs/architecture`
- use C4 levels as the organizing frame
- describe real repo modules and real external systems

Phase 2, later if justified:
- introduce a single checked-in architecture model for generation
- evaluate whether that model should be C4InterFlow YAML or Structurizr DSL
- add CI generation only after the model is stable enough to be worth automating

## Why Not Full C4InterFlow Today

- It adds another toolchain before we have a stable architecture corpus.
- The repo does not yet need flow-heavy or query-heavy architecture modelling.
- A bad or incomplete generated model would be worse than a small, accurate manual baseline.

## Exit Criteria For Phase 2

Move from Markdown-only to generated architecture artifacts when at least one of these becomes true:
- the service splits into multiple deployable applications
- component/dependency churn makes manual docs expensive
- contributors need generated diagrams in CI or GitHub Pages
- architecture queries and flow documentation become routine
