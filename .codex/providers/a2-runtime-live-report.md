# A.2 Runtime Live Validation Report

- Generated at (UTC): `2026-02-09T21:35:41Z`
- Strict mode: `true`
- Overall status: `fail`
- Scenarios: pass=5 skip=1 fail=0 total=6
- Modules: total=21

## Scenario Results

| Scenario | Status | Modules | Detail |
| --- | --- | --- | --- |
| `S1` | `pass` | `RK-02, RK-03, RK-04, RK-19, RK-21` | proposal_event=evt-a2-live-s1-prelude plan_hash=f1f5e10e4a39772ad3d2a8e76d1e4ec04d8b9f88805377492de2b7e8bd0b6ac9 generated_shed_event=evt/sess-a2-live-s1/turn-a2-live-s1/000001 |
| `S2` | `pass` | `RK-05, RK-16, RK-22, RK-23` | transport_signal=connected cancel_fence_signal=playback_cancelled |
| `S3` | `pass` | `RK-06, RK-07, RK-25, RK-26` | node_order=node-stt,node-llm pool_submitted=2 shed_signal=shed |
| `S4` | `pass` | `RK-08, RK-12, RK-13, RK-14, RK-17` | failure_signals=3 pressure_signals=4 |
| `S5` | `skip` | `RK-10, RK-11` | no enabled providers executed |
| `S6` | `pass` | `RK-24` | terminal_state=Closed decision=deauthorized_drain |

## Module Results

| Module | Scenario | Status | Detail |
| --- | --- | --- | --- |
| `RK-02` | `S1` | `pass` | proposal_event=evt-a2-live-s1-prelude plan_hash=f1f5e10e4a39772ad3d2a8e76d1e4ec04d8b9f88805377492de2b7e8bd0b6ac9 generated_shed_event=evt/sess-a2-live-s1/turn-a2-live-s1/000001 |
| `RK-03` | `S1` | `pass` | proposal_event=evt-a2-live-s1-prelude plan_hash=f1f5e10e4a39772ad3d2a8e76d1e4ec04d8b9f88805377492de2b7e8bd0b6ac9 generated_shed_event=evt/sess-a2-live-s1/turn-a2-live-s1/000001 |
| `RK-04` | `S1` | `pass` | proposal_event=evt-a2-live-s1-prelude plan_hash=f1f5e10e4a39772ad3d2a8e76d1e4ec04d8b9f88805377492de2b7e8bd0b6ac9 generated_shed_event=evt/sess-a2-live-s1/turn-a2-live-s1/000001 |
| `RK-05` | `S2` | `pass` | transport_signal=connected cancel_fence_signal=playback_cancelled |
| `RK-06` | `S3` | `pass` | node_order=node-stt,node-llm pool_submitted=2 shed_signal=shed |
| `RK-07` | `S3` | `pass` | node_order=node-stt,node-llm pool_submitted=2 shed_signal=shed |
| `RK-08` | `S4` | `pass` | failure_signals=3 pressure_signals=4 |
| `RK-10` | `S5` | `skip` | no enabled providers executed |
| `RK-11` | `S5` | `skip` | no enabled providers executed |
| `RK-12` | `S4` | `pass` | failure_signals=3 pressure_signals=4 |
| `RK-13` | `S4` | `pass` | failure_signals=3 pressure_signals=4 |
| `RK-14` | `S4` | `pass` | failure_signals=3 pressure_signals=4 |
| `RK-16` | `S2` | `pass` | transport_signal=connected cancel_fence_signal=playback_cancelled |
| `RK-17` | `S4` | `pass` | failure_signals=3 pressure_signals=4 |
| `RK-19` | `S1` | `pass` | proposal_event=evt-a2-live-s1-prelude plan_hash=f1f5e10e4a39772ad3d2a8e76d1e4ec04d8b9f88805377492de2b7e8bd0b6ac9 generated_shed_event=evt/sess-a2-live-s1/turn-a2-live-s1/000001 |
| `RK-21` | `S1` | `pass` | proposal_event=evt-a2-live-s1-prelude plan_hash=f1f5e10e4a39772ad3d2a8e76d1e4ec04d8b9f88805377492de2b7e8bd0b6ac9 generated_shed_event=evt/sess-a2-live-s1/turn-a2-live-s1/000001 |
| `RK-22` | `S2` | `pass` | transport_signal=connected cancel_fence_signal=playback_cancelled |
| `RK-23` | `S2` | `pass` | transport_signal=connected cancel_fence_signal=playback_cancelled |
| `RK-24` | `S6` | `pass` | terminal_state=Closed decision=deauthorized_drain |
| `RK-25` | `S3` | `pass` | node_order=node-stt,node-llm pool_submitted=2 shed_signal=shed |
| `RK-26` | `S3` | `pass` | node_order=node-stt,node-llm pool_submitted=2 shed_signal=shed |

## Provider Outcomes

| Provider | Modality | Status | Outcome | Retry | Attempts | Reason |
| --- | --- | --- | --- | --- | --- | --- |
| `stt-deepgram` | `stt` | `skip` | `` | `` | `0` | RSPP_STT_DEEPGRAM_ENABLE!=1 |
| `stt-google` | `stt` | `skip` | `` | `` | `0` | RSPP_STT_GOOGLE_ENABLE!=1 |
| `stt-assemblyai` | `stt` | `skip` | `` | `` | `0` | RSPP_STT_ASSEMBLYAI_ENABLE!=1 |
| `llm-anthropic` | `llm` | `skip` | `` | `` | `0` | RSPP_LLM_ANTHROPIC_ENABLE!=1 |
| `llm-gemini` | `llm` | `skip` | `` | `` | `0` | RSPP_LLM_GEMINI_ENABLE!=1 |
| `llm-cohere` | `llm` | `skip` | `` | `` | `0` | RSPP_LLM_COHERE_ENABLE!=1 |
| `tts-elevenlabs` | `tts` | `skip` | `` | `` | `0` | RSPP_TTS_ELEVENLABS_ENABLE!=1 |
| `tts-google` | `tts` | `skip` | `` | `` | `0` | RSPP_TTS_GOOGLE_ENABLE!=1 |
| `tts-amazon-polly` | `tts` | `skip` | `` | `` | `0` | RSPP_TTS_POLLY_ENABLE!=1 |
