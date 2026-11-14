# Performance acceptance contract

The product goal is to outperform every applicable existing PHP compiler and execution engine. This is not a result already achieved by this repository.

## Scope and discovery

The [competitor catalog](../data/competitor-catalog.json) is a seed for ongoing discovery, not an exhaustive or source-locked inventory. Distinguish independent engines from website deployment stacks. Include suitable historical PHP implementations and current alternatives. A project that cannot execute a workload is not a performance loss; compare its supported common workloads separately.

## Fair comparisons

Freeze workloads, inputs, resource budgets, correctness checks, security mode, build flags, cache state, primary metrics, and statistical rules before tuning. Use genuine applications, engine-level workloads, holdout workloads, cold/hot runs, concurrency sweeps, overload, and recovery. Never modify application semantics or disable protections to gain speed.

Measure equivalent successful work. Compare latency at equal arrival rates and throughput at the same latency/error budget. Record compilation, startup, warm-up, memory, CPU, artifact size, and external-service costs separately. Keep execution-core and deployment-stack results distinct.

## Decision rule

The initial protocol requires at least 30 independent paired experimental repetitions, randomized or interleaved ordering, sufficient request samples for tail quantiles, and a preregistered sample-size/stop rule. Use 95% confidence intervals with an appropriate multiple-comparison correction and account for dependencies in the experiment.

Latency advantage is competitor latency divided by Phego latency. Throughput advantage is Phego correct throughput divided by competitor correct throughput. A required comparison passes only when the corrected interval's lower bound is strictly greater than 1. Ties, insufficient data, unsupported workloads, and failed correctness checks are not wins. A geometric mean cannot erase a losing required cell.

Every applicable required competitor/workload cell must pass to satisfy the complete performance goal. Publish the exact tested scope and date rather than claiming a proof over all conceivable programs and future engines.

## Evidence

Retain raw observations, commands, source/configuration locks, input seeds, correctness results, resource measurements, analysis code, and an independent reproduction. The authoritative methodology is [chapter 37](../spec/specification.zh-TW.md#chapter-37).
