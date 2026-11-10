# Product

<!-- impeccable:product-schema 1 -->

## Platform

terminal

(Not one of web/ios/android/adaptive: anchors is a Go terminal UI built with Bubble Tea and Lip Gloss, rendered in the user's terminal emulator.)

## Users

Both developers running Docker and local clusters (kind, minikube) on their own machine, and DevOps/SRE people operating real hosts and clusters. The author is the first user: it's a portfolio project that they will also use professionally at work.

## Product Purpose

One terminal tool to manage, inspect, and interact with Docker containers and Kubernetes pods: list them, see their state, and act on them without switching between `docker` and `kubectl`.

## Positioning

Open decision. The confirmed goal is beauty and feel: anchors should be a polished, animated, good-looking TUI. A differentiating mechanism against lazydocker or k9s hasn't been chosen yet.

## Operating Context

Runs in a terminal next to the user's other dev and ops work. Talks to the local Docker daemon (respects `DOCKER_HOST`) and, later, to Kubernetes through kubeconfig.

## Capabilities and Constraints

- Today: listing Docker containers (ID, name, ports, state, status, health).
- Planned: container actions, logs, exec, stats; Kubernetes pods.
- Must work at 80x24 minimum and assume a truecolor terminal, degrading gracefully otherwise.
- Bubble Tea v1, Bubbles, Lip Gloss, Huh, Glamour. New libraries must be proposed to the author before they're added.

## Brand Commitments

- Name: **anchors**. Nautical imagery (ocean, anchors, ships) is the author's chosen theme.
- Opening screen: a pixel-art / bit-dither title screen that waits for Enter.

## Evidence on Hand

No logo, screenshots, users, or benchmarks exist yet. Don't invent any.

## Product Principles

1. Looks and feels exceptional: motion and color are part of the product.
2. Fast and keyboard-first in daily use. The look never gets in the way of the task.
3. Docker and Kubernetes sit side by side with a consistent language.
