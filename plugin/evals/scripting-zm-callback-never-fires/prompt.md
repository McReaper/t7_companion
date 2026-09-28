---
description: zombies doesn't dispatch the shared actor callbacks
tags: [trigger]
max_turns: 4
allowed_tools: [Skill, Read, Glob, Grep]
---

i registered callback::on_actor_killed(&my_func) in my zombies map to count kills but my_func never runs. no error in the log. whats wrong
