# Recipe: Explore an Idea

> *"Way she goes. Sometimes she goes, sometimes she doesn't."*

---

## The Situation

You have a vague product idea. You need to flesh it out — understand the problem space, identify users, map out the solution.

## The Command

```
/bubbles.workflow  product-discovery for my-idea
```

If you want the system to interview you first instead of inferring the missing context autonomously:

```
/bubbles.workflow  product-discovery for my-idea socratic: true socraticQuestions: 5
```

**Phases:** analyze → ux

## Or Step by Step

### 1. Business Analysis

```
/bubbles.analyst  I want to build a marketplace for vintage furniture
```

The analyst will:
- Research the problem space
- Identify actors and use cases
- Do competitive analysis
- Write a spec with requirements

### 2. UX Design

```
/bubbles.ux  create user flows for the furniture marketplace
```

The UX agent will:
- Create user personas
- Design user flows
- Build wireframe descriptions
- Define interaction patterns

## What You Get

A clear understanding of **what** to build and **for whom**, with artifacts ready for the design phase when you're ready.
