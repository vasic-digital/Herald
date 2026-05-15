# Herald

Ingesting system events and reliably fanning them out to multiple notification channels so every alert reaches the right destination without confusion.

All exiting projetc upstreams:

- GitHub: `git@github.com:vasic-digital/Herald.git` (main repository)
- GitLab: `git@gitlab.com:vasic-digital/herald.git`
- GitFlic: `git@gitflic.ru:vasic-digital/herald.git`
- GitFlic: `git@gitverse.ru:vasic-digital/Herald.git`

## What Herald can (must be able to) do?

Herald is the mechanism which receives input from the source and sends it to one or more destinations (implemented channels). Depending on the implementation (Flavor) of the Herald sources and destinations could be various. We can have single type or multiple type inputs and same applies for the outputs.

For example, input can be the result of execution of some pipeline. Heralds is then sending this result (some report for example) to certain output (or outputs). It could be, for a sake of illustration, a messaging system which notifies subscribers.

The Possibilities are not limited!

The structure of the System MUST be hierarchical so in the top levels (closest to the root) we have abstractions and base reusable, main shared mechanisms, shared codebase, while inside the `flavors` directory we MUST have all flavors - the implementations.

*Note:* We MUST NOT be obligated to follow this structure as many parent project's specific custom flavors may need to exist! They could be private and require safe location in the System or project. We MUST make sure that such flexibility is possible! This MUST BE fully supported!

## How Herald should achieve its mission objectives?

Every herald flavor will be individual binnary which users can add to the System path throguh the `.bashrc` or `.zshrc` (for example). Each Herald flavor will have shared set of commands and parameters while there will be as well commands and parameters specific to particular Herald flavors (implementations).

Herald applications are CLI binarries which are mainly designed for CI integration and various Pipelines. They can be easily incorporated for use with various AI CLI Agents (Claude Code, OpenCode and others...) as well or other similar use cases (triggering by Crond - cron jobs, and so on ...).

Some Herald application names can be: `pherald` for the `Project Herald`, `sherald` for the `System Herald` and so on.

### Technology stack

Herald project and all flavors MUST BE writtn in Go.

## Commons

The following paragraphs define shared functionality and implementations among all Flavors of Herald.

The `commons` will contain the most generic abstractions and shared implementations which will be later inherited through the inheritance hierachy. 

Example:

`commons -> commons level 1 -> commons level 2 -> ... -> commons level N -> Flavor`

### Common Messeging Herald

Common Messeging Herald (`commons_messaging`) is the `commons level 1` abstraction layer. Every Messeging Herald Flavor MUST offer support for several main integrations:

- Telegram
- Slack
- Max (max.ru)
- Email
- Markdown document with PDF and HTML export

For each of Messaging services user MUST provide required tokens, credentials or API keys depending on the platform. All details MUST BE documented in proper user guides and manuals with step by step instructions so users can easily obtain and provide required information.

All sensitive data like credentials, tokens or API keys MUST be in inside proper `.env` file (create for the documentation purposes `.env.example` file to illustrate everything that users can put there) which MUST BE Git ignored! System MUST BE capable to obtain all these environment variables from exported variables from `.bashrc` and `.zshrc` or any other System profile script which can export the variables. Using values fron `.env` comes after top level ones are loaded and everything defined inside the `.env` file will override them (if same variabels are defined there).

Everything that is sent or received through any of the interated Messengers channels such as Telegram, Slack, Max and others (Email as well) will be stored inside main Markdown file and exported into PDF and HTML regularly. Markdown file and its exports MUST BE always in sync! Location of the Markdown file inside the Project MUST BE the following: `docs/herald/diary/main.md` (`main.pdf` and `main.html`).

### APIs

We MUST perform in depth research and bring int all required APIs and SDKs required for each Messaging solution to be fully incorporated. We MUST perform deep web research and obtain information about API documentation and SDKs (Go). Every SDK and API which are available as Git repositories MUST BE incorporated as Git Submodules into the project. Example path for the Submodule(s): `commons_messaging/api/telegram` or `commons_messaging/sdk/telegram`.

## Flavors (the implementations)

Main Go abstractions and shared codebase (with shared implementations) will be used as the base which Flavors will inherit and build on top of it.

### Project Herald

The Project Herald is focused on Projects and its development. All Projects share some commons and Project Herald MUST FIT as the universal player here!

What are the specifics that Project Herald is having and others do not?

### System Herald

Tbd

What are the specifics that System Herald is having and others do not?

### Others and misc

Here we will list main ideas for upcoming Flavors which MUST BE planned with proper deep web research and fully implemented:

- Tbd

## Integration into the Constitution

Tbd

## Documentation

Make sure the main `README` document is fully updated with all relevant project details and all user guides and manuals are properly linked (and other documentation and relevant materials too) to it!

We MUST have all mandatory documentation up to the smallest details, full user guides and manuals, diagrams and scheme in all major formats and other relevant materials users may need.

## Testing

Whole project and all of its derrivates MUST follow testing rules from our root Constitution, CLAUDE.MD and AGENTS.MD (constitution Submodule from the main parent project).

## Notes

Tbd
