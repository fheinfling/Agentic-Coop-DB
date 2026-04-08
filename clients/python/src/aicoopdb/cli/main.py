"""aicoopdb — top-level CLI.

Subcommands:

    ai-coop-db init       interactive onboarding wizard (start the stack, mint a key)
    ai-coop-db sql        run a one-shot SQL statement
    ai-coop-db me         show the calling key's workspace + role
    ai-coop-db key        manage API keys (create, rotate)
    ai-coop-db queue      inspect / flush the local offline retry queue
    ai-coop-db doctor     verify that everything is wired correctly
"""

from __future__ import annotations

import json
from typing import Any

import typer

from aicoopdb import connect
from aicoopdb.cli import config as cli_config
from aicoopdb.cli.doctor import doctor as doctor_cmd
from aicoopdb.cli.init import init as init_cmd
from aicoopdb.cli.key import key_app
from aicoopdb.cli.queue import queue_app

app = typer.Typer(
    name="aicoopdb",
    help="AI Coop DB CLI — auth gateway client for shared PostgreSQL.",
    no_args_is_help=True,
)

app.add_typer(key_app, name="key", help="Manage API keys.")
app.add_typer(queue_app, name="queue", help="Inspect / flush the local offline retry queue.")
app.command(name="init", help="Interactive onboarding wizard.")(init_cmd)
app.command(name="doctor", help="Verify config, network, auth, db, and migrations.")(doctor_cmd)


@app.command()
def me() -> None:
    """Print { workspace, role, server } for the configured key."""
    cfg = _require_config()
    db = connect(cfg.base_url, api_key=cfg.api_key)
    typer.echo(json.dumps(db.me(), indent=2))


@app.command()
def sql(
    statement: str = typer.Argument(..., help="The SQL statement (use $1, $2, ... for params)"),
    param: list[str] = typer.Option([], "--param", "-p", help="Repeatable; positional values for $1, $2, ..."),
) -> None:
    """Run a one-shot SQL statement against the configured server."""
    cfg = _require_config()
    db = connect(cfg.base_url, api_key=cfg.api_key)
    parsed_params: list[Any] = list(param)
    res = db.execute(statement, parsed_params)
    typer.echo(
        json.dumps(
            {
                "command": res.command,
                "columns": res.columns,
                "rows": res.rows,
                "rows_affected": res.rows_affected,
                "duration_ms": res.duration_ms,
            },
            indent=2,
        )
    )


def _require_config() -> cli_config.CLIConfig:
    cfg = cli_config.load()
    if cfg is None:
        typer.echo("no config found — run `ai-coop-db init` first", err=True)
        raise typer.Exit(code=2)
    return cfg


if __name__ == "__main__":
    app()
