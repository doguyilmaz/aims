---
title: aims
layout: hextra-home
---

{{< hextra/hero-badge >}}
  <div class="hx:w-2 hx:h-2 hx:rounded-full hx:bg-primary-400"></div>
  One binary · macOS, Linux &amp; Windows · Claude Code &amp; Codex
{{< /hextra/hero-badge >}}

<div class="hx:mt-6 hx:mb-6">
{{< hextra/hero-headline >}}
  Your personal and work accounts,&nbsp;<br class="hx:sm:block hx:hidden" />side by side
{{< /hextra/hero-headline >}}
</div>

<div class="hx:mb-12">
{{< hextra/hero-subtitle >}}
  aims (AI multi-session) gives each Claude Code and Codex account its own login and shares everything else.&nbsp;<br class="hx:sm:block hx:hidden" />Switch without logging out, continue a conversation on another account, and move on when one hits its limit.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx:mb-6">
{{< hextra/hero-button text="Get started" link="docs/" >}}
</div>

<div class="hx:mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="No more logout, login"
    subtitle="Each account logs in once, into its own folder. `aims use work` switches new sessions; `aims claude@work` starts one on that account right away."
  >}}
  {{< hextra/feature-card
    title="One history across accounts"
    subtitle="Conversations, settings, skills and MCP servers are shared, so `aims claude --continue` picks up where the other account stopped. Work history can stay separate instead."
  >}}
  {{< hextra/feature-card
    title="Failover on limits"
    subtitle="When an account is out of quota or its login died, the next session goes to the next account. Headless runs retry on their own when the failed attempt changed nothing."
  >}}
  {{< hextra/feature-card
    title="Usage at a glance"
    subtitle="Plan usage from Claude Code's status line and the Codex account API, shown as bars in `aims status` and in the dashboard, so aims switches before a limit hits."
  >}}
  {{< hextra/feature-card
    title="The AI can switch too"
    subtitle="An MCP server and a skill let Claude Code and Codex check the accounts, fail over, and hand a task to another account in the middle of a session."
  >}}
  {{< hextra/feature-card
    title="Logins stay put"
    subtitle="Logins are never copied or uploaded. aims points each tool at the right folder and starts it; the tools talk to their providers as they always do."
  >}}
{{< /hextra/feature-grid >}}
