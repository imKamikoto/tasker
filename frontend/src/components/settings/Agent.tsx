import { useState } from "react";

import type { Paths, Stats } from "../../api";
import { shortDate } from "../../format";
import type { UISettings } from "../../settings";
import { Card, Fact, PathLine, Row, Toggle } from "./controls";

type Props = {
  settings: UISettings;
  onChange: (patch: Partial<UISettings>) => void;
  paths: Paths | null;
  stats: Stats | null;
  vault: string;
};

export function Agent({ settings, onChange, paths, stats, vault }: Props) {
  const [copied, setCopied] = useState(false);

  const mcp = paths?.MCP ?? "";
  // Готовый кусок конфига: набирать его руками — верный способ ошибиться в
  // пути и потом искать, почему Claude Code не видит инструментов.
  const config = JSON.stringify(
    {
      mcpServers: {
        tasker: { command: mcp || "<путь к tasker-mcp>", args: ["--vault", vault] },
      },
    },
    null,
    2,
  );

  const copy = () => {
    void navigator.clipboard.writeText(config).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <>
      <Card>
        <Row label="Что делал агент">
          <div className="facts">
            <Fact label="От агента" value={stats ? stats.Agent : "…"} />
            <Fact
              label="Последняя"
              value={
                stats && stats.AgentLast && !stats.AgentLast.startsWith("0001")
                  ? shortDate(stats.AgentLast)
                  : "не писал"
              }
            />
          </div>
        </Row>

        <Row label="Метка" hint="Ромб, полоса слева и бейдж AGENT в списке">
          <Toggle
            checked={settings.agentBadge}
            label="Отмечать заметки, заведённые агентом"
            onChange={(agentBadge) => onChange({ agentBadge })}
          />
        </Row>
      </Card>

      <Card>
        <Row label="MCP-сервер" hint="Отдельный бинарник, работает и при закрытом приложении">
          <div className="stack">
            {mcp ? (
              <PathLine path={mcp} />
            ) : (
              <span className="card__note">
                Рядом с приложением его нет. Соберите: <code>go build -o bin/tasker-mcp ./cmd/tasker-mcp</code>
              </span>
            )}
          </div>
        </Row>

        <Row label="Конфиг для Claude Code">
          <div className="stack">
            <pre className="codeblock">{config}</pre>
            <div className="row-actions">
              <button className="button" onClick={copy}>
                {copied ? "Скопировано" : "Скопировать"}
              </button>
            </div>
          </div>
        </Row>
      </Card>

      <p className="section-note">
        Что агенту нельзя: удалять насовсем, переименовывать пачкой, писать в{" "}
        <code>config.json</code>, трогать git напрямую и читать за пределами хранилища. Это границы
        MCP-сервера, а не настройка — выключателя у них нет.
      </p>
    </>
  );
}
