import { useState } from "react";

import { api, describeError, type Stats } from "../../api";
import { fileSize, noteCount } from "../../format";
import { limits, type UISettings } from "../../settings";
import { Card, Fact, PathLine, Row, Slider, Toggle } from "./controls";

type Props = {
  settings: UISettings;
  onChange: (patch: Partial<UISettings>) => void;
  vault: string;
  recent: string[];
  stats: Stats | null;
  onStats: (stats: Stats) => void;
  onVaults: () => void;
  /** Ведётся ли история. Живёт в Go рядом с заметками, не в config.json. */
  gitEnabled: boolean;
  onGitEnabled: (enabled: boolean) => void;
  onError: (message: string) => void;
};

export function Storage({
  settings,
  onChange,
  gitEnabled,
  onGitEnabled,
  vault,
  recent,
  stats,
  onStats,
  onVaults,
  onError,
}: Props) {
  // Смена папки записана, но ещё не применена: до перезапуска приложение
  // работает со старой, и молчать об этом нельзя.
  const [pending, setPending] = useState<string | null>(null);
  const [rebuilding, setRebuilding] = useState(false);

  const choose = () => {
    api
      .chooseVault()
      .then((picked) => {
        setPending(picked);
        onVaults();
      })
      .catch((err) => {
        const message = describeError(err);
        // Закрытый диалог — это не ошибка, а отказ от действия.
        if (!message.includes("no vault chosen")) onError(message);
      });
  };

  const switchTo = (path: string) => {
    api
      .switchVault(path)
      .then(() => {
        setPending(path);
        onVaults();
      })
      .catch((err) => onError(describeError(err)));
  };

  const rebuild = () => {
    setRebuilding(true);
    api
      .rebuildIndex()
      .then(onStats)
      .catch((err) => onError(describeError(err)))
      .finally(() => setRebuilding(false));
  };

  return (
    <>
      <Card>
        <Row label="Папка с заметками" hint="Обычные .md файлы, править можно чем угодно ещё">
          <div className="stack">
            <PathLine path={vault} onReveal={() => void api.revealVault(vault)} />
            <div className="row-actions">
              <button className="button" onClick={choose}>
                Выбрать другую…
              </button>
            </div>
            {pending && pending !== vault && (
              <div className="card__warn">
                Выбрана <code>{pending}</code>. Приложение откроет её при следующем запуске.
                <button className="button button--accent" onClick={() => void api.restart()}>
                  Перезапустить
                </button>
              </div>
            )}
          </div>
        </Row>

        {recent.length > 0 && (
          <Row label="Недавние">
            <div className="stack">
              {recent.map((path) => (
                <div key={path} className="pathline">
                  <button className="button button--link pathline__path" onClick={() => switchTo(path)}>
                    {path}
                  </button>
                  <button className="button button--link" onClick={() => void api.forgetVault(path)}>
                    забыть
                  </button>
                </div>
              ))}
            </div>
          </Row>
        )}
      </Card>

      <Card>
        <Row label="Что внутри">
          <div className="facts">
            <Fact label="Заметок" value={stats ? stats.All : "…"} />
            <Fact label="В корзине" value={stats ? stats.Trashed : "…"} />
            <Fact label="От агента" value={stats ? stats.Agent : "…"} />
            <Fact label="Ноутбуков" value={stats ? stats.Notebooks : "…"} />
            <Fact label="Тегов" value={stats ? stats.Tags : "…"} />
            <Fact label="Индекс" value={stats ? fileSize(stats.IndexSize) : "…"} />
          </div>
        </Row>

        <Row
          label="Индекс"
          hint="Производный: правда в файлах, пересборка всегда безопасна"
        >
          <div className="stack">
            <div className="row-actions">
              <button className="button" disabled={rebuilding} onClick={rebuild}>
                {rebuilding ? "Пересобираю…" : "Пересобрать"}
              </button>
            </div>
            <span className="card__note">
              Нужна, если vault правили другими инструментами при закрытом приложении и список
              разошёлся с диском.
            </span>
          </div>
        </Row>
      </Card>

      <Card>
        <Row label="История" hint="Версии заметок в git внутри самой папки, без сервера">
          <div className="stack">
            <Toggle
              checked={gitEnabled}
              label="Вести историю изменений"
              onChange={(enabled) => {
                // Ответ идёт в Go: это свойство папки, а не интерфейса, и его
                // читает ещё и tasker-mcp. Не вышло — возвращаем тумблер, а не
                // оставляем его показывать несуществующее состояние.
                onGitEnabled(enabled);
                api
                  .setGitEnabled(enabled)
                  // Включённой истории надо вернуть окно из настроек:
                  // репозиторий только что открылся и про него не знает.
                  .then(() => (enabled ? api.configureGit(settings.commitWindow) : undefined))
                  .catch((err) => {
                    onGitEnabled(!enabled);
                    onError(describeError(err));
                  });
              }}
            />
            <span className="card__note">
              {gitEnabled
                ? "Каждая правка уезжает в git внутри папки с заметками. Никуда не отправляется: репозиторий локальный."
                : "Хранилище — просто папка с файлами. Выключение ничего не удаляет: уже накопленные коммиты остаются на месте и вернутся, если включить обратно."}
            </span>
          </div>
        </Row>
      </Card>

      {gitEnabled && (
        <Card>
          <Row
            label="Окно автокоммита"
            hint="Через сколько правки уезжают в историю git"
          >
            <div className="stack">
              <Slider
                value={settings.commitWindow}
                min={limits.commitWindow.min}
                max={limits.commitWindow.max}
                step={limits.commitWindow.step}
                format={(value) => (value === 0 ? "сразу" : `${value} с`)}
                onChange={(commitWindow) => onChange({ commitWindow })}
              />
              <span className="card__note">
                {settings.commitWindow === 0
                  ? "Каждое сохранение — отдельный коммит. История подробная, но длинная."
                  : `Правки собираются в один коммит. Файлы всё это время уже на диске: git здесь история, а не хранилище.`}
              </span>
              {settings.commitWindow > 0 && (
                <div className="row-actions">
                  <button className="button" onClick={() => void api.commitNow()}>
                    Закоммитить сейчас
                  </button>
                </div>
              )}
            </div>
          </Row>
        </Card>
      )}

      {stats && stats.Trashed > 0 && (
        <p className="section-note">
          В корзине {noteCount(stats.Trashed)}. Автоочистки нет: удалить насовсем можно только
          вручную из корзины.
        </p>
      )}
    </>
  );
}
