# Lens: user — The overseer under stress

## Framing

The overseer's experience during an infrastructure failure is not a debugging
problem — it is a loss-of-control moment. When Dolt crashes or disk fills,
the overseer does not primarily need information. They need the bleeding to
stop. Every second of delay between "I see the failure" and "all agents are
paused" is another API call burning money and potentially corrupting state.
The current system forces the overseer to perform a manual, sequential,
error-prone ritual (find session, kill session, repeat) while under cognitive
load and time pressure. The UX of the crisis IS the crisis.

## What other lenses will likely miss

Technical lenses will focus on detection accuracy, blast radius, and recovery
sequencing. They will likely design for the ideal case: the system detects the
failure automatically and acts before the overseer even knows. But this misses
the psychological gap. An overseer who did not witness the system stop the
agents themselves will not trust that it did. They will check. They will
second-guess. They may override a correct automatic stop because the visual
feedback did not make the action feel real and completed. Trust in automation
is earned through legible action, not just correct outcomes. The system must
make its own decisions visible in a way the overseer can audit in under five
seconds, while panicked.

## Proposed solution

A single-command hard stop with an immediate, unambiguous visual receipt:

`gt estop` pauses all agent sessions (SIGTSTP to tmux panes, not kill — so
state is preserved), writes a timestamped freeze record, and within two
seconds repaints every tmux status bar red with "FROZEN 14:23:07". The
overseer sees the change happen. They do not need to check each session. The
receipt is the proof. Unfreeze is a separate, intentional command (`gt
estop --resume`) that requires an explicit argument, preventing accidental
recovery. The freeze record includes which sessions were caught and their
last-known task so the overseer can triage without reopening each pane.

## Failure mode

The overseer uses `gt estop` preemptively during ambiguous situations — slow
responses, a suspicious log line — and then forgets to resume. Agents sit
frozen for hours. Work is lost. The overseer, burned by this, stops trusting
the freeze state and develops a habit of hard-killing sessions instead, which
destroys the state preservation benefit entirely. Overuse driven by low cost
of activation degrades the tool's value and trains the overseer away from the
softer, recoverable path.

## Cheap experiment

Before building anything: next time a real or simulated failure occurs, have
the overseer narrate aloud what they are doing and what they wish existed at
each step. Time the gap between "I notice something is wrong" and "all agents
are stopped." Record the sequence of actions and where attention fractured.
This costs nothing and will reveal whether the bottleneck is discovery,
execution, or confirmation — each of which implies a different design.
