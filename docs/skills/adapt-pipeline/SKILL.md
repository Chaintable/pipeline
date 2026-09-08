---
name: adapt-pipeline
description: "Adapt a blockchain execution client to integrate Pipeline. Detects client type (Geth v1.14+, legacy Geth, Reth) and guides step-by-step implementation."
user-invocable: true
argument-hint: "<path-to-client-repo>"
---

# Pipeline Adaptation Entry Point

You are an expert at integrating the Pipeline data processing system into blockchain execution clients. Your job is to detect the client type and route to the appropriate specialized adaptation skill.

## Step 1: Validate Input

The target client repository path is: `$ARGUMENTS`

If no path is provided, ask the user:
> Please provide the path to the client repository you want to adapt. Example: `/home/user/go-ethereum`

## Step 2: Detect Client Type

Perform the following detection steps **in order**:

### 2.1 Check for Rust (Reth)
- Search for `Cargo.toml` in the root of `$ARGUMENTS`
- If found, search for `reth` or `revm` in the workspace dependencies
- If confirmed → **Reth client detected**

### 2.2 Check for Go (Geth family)
- Search for `go.mod` in the root of `$ARGUMENTS`
- If not found → report error: "Not a recognized blockchain client (no Cargo.toml or go.mod found)"

### 2.3 Distinguish Standard vs Legacy Geth
Search in the `$ARGUMENTS` codebase for these patterns:

**Standard Geth indicators** (any of these → standard):
- `tracing.Hooks` struct in `core/tracing/hooks.go`
- `tracers.LiveDirectory` in `eth/tracers/`
- `OnBlockEnd` field in a `Hooks` struct

**Legacy Geth indicators** (any of these → legacy):
- `vm.EVMLogger` interface in `core/vm/`
- `vm.Logger` interface
- `CaptureStart` / `CaptureEnd` methods in a tracer interface
- No `core/tracing/hooks.go` file

## Step 3: Report Detection Result

Report the detection findings clearly:

```
## Detection Result

- **Repository**: <path>
- **Language**: Go / Rust
- **Client Type**: Standard Geth (v1.14+) / Legacy Geth / Reth
- **Evidence**: <what was found>
- **Recommended Skill**: adapt-pipeline-geth / adapt-pipeline-legacy / adapt-pipeline-reth
```

## Step 4: Check for Existing Adaptation

Before routing, check if the client already has Pipeline integration:
- Search for `pipeline` in `go.mod` or `Cargo.toml` dependencies
- Search for `debankBlock` or `debank_block` in the codebase
- Search for `PipelineTracer` or `pipeline_tracer`

If found, warn the user:
> This client appears to already have Pipeline integration. Proceeding may cause conflicts. Do you want to continue?

## Step 5: Route to Specialized Skill

Based on the detection result, invoke the corresponding skill:

- **Standard Geth** → Use `/adapt-pipeline-geth $ARGUMENTS`
- **Legacy Geth** → Use `/adapt-pipeline-legacy $ARGUMENTS`
- **Reth** → Use `/adapt-pipeline-reth $ARGUMENTS`

Tell the user which skill you're routing to and why, then invoke it.
