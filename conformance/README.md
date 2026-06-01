# Conformance

This directory defines protocol scenarios that every language client must satisfy.

The v0 clients currently keep small language-local fake servers in their tests so
each package can run independently. The JSON scenarios here are the shared truth
source for those fake servers and for future cross-language conformance runners.

## Scenario Format

Each scenario contains ordered client requests and expected server messages. A
client passes the scenario when its public API produces the expected session
objects, event stream, and protocol errors while interacting with a server that
emits the listed messages.

## v0 Scenarios

- `fixtures/v0-basic-prompt.json`: initialize, create a session, prompt once,
  receive turn events, and shut down.
- `fixtures/v0-protocol-error.json`: initialize, then map a protocol error from
  `createSession` into the language's idiomatic protocol error type.

## v1 Follow-Ups

- Generate fake servers for Node, Python, and Go from these fixtures.
- Add explicit timing coverage for `turn.started` before prompt response.
- Add server-crash transport error fixtures.
- Add `steer`, `cancel`, `getStatus`, `listSessions`, and `resumeSession`
  fixtures.

