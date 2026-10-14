# Corrected App qualification — PR 15

The [CI run](https://github.com/sofianbll/bifrost/actions/runs/38549247160) passed on Bifrost commit `f54038b339a0d63b66dfa85158a585938c328bbd` with Excalidraw commit `4c3700f0ef25a261177fecad03284a57bf7847be`. The [manifest](manifest.json) records image digest, version, route, checkpoints, fixed camera, viewport and artifact hashes.

The real-App wire checks pass 80/80, rendered checks 18/18 and the actual-library normalizer 10/10. Eager and delayed initialization of the actual built App both render all three labels and call `read_checkpoint`. The +24/-20 rendered displacement remains rejected. Both architecture builds and MCP tests, workflow lint and Dependency Review pass; publication is skipped.

The before/after/offset captures contain only the synthetic drawing and its App controls. Full HTML, credentials, host console and private logs are excluded. These are automated browser results. The [final corrected native build](../native-codex-20261014/final/README.md) separately passes loaded-artifact, real save/read/restore and same-scene framing checks. Initialization screenshots capture the draw animation and are not used as alignment measurements.
