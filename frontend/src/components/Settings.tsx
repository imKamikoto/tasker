import { useEffect, useState } from "react";

import { api, describeError, type Build, type Paths, type Stats } from "../api";
import type { Keymap } from "../keys";
import type { UISettings } from "../settings";
import { About } from "./settings/About";
import { Agent } from "./settings/Agent";
import { Appearance } from "./settings/Appearance";
import { EditorPrefs } from "./settings/EditorPrefs";
import { Shortcuts } from "./settings/Shortcuts";
import { Storage } from "./settings/Storage";

/** Разделы в порядке макета T6. */
const sections = [
  { id: "appearance", label: "Внешний вид" },
  { id: "storage", label: "Папка и файлы" },
  { id: "editor", label: "Редактор" },
  { id: "shortcuts", label: "Шоткаты" },
  { id: "agent", label: "Агент" },
  { id: "about", label: "О программе" },
] as const;

type Section = (typeof sections)[number]["id"];

type Props = {
  settings: UISettings;
  onChange: (patch: Partial<UISettings>) => void;
  keymap: Keymap;
  onKeymap: (keymap: Keymap) => void;
  onResetKeymap: () => void;
  onClose: () => void;
};

/**
 * Settings — экран настроек.
 *
 * Оверлей поверх главного окна, а не второе окно: настройки применяются сразу,
 * и двум окнам пришлось бы синхронизировать состояние событием ради экрана,
 * который открывают раз в месяц. Размеры и раскладка — из макета T6.
 */
export function Settings({
  settings,
  onChange,
  keymap,
  onKeymap,
  onResetKeymap,
  onClose,
}: Props) {
  const [section, setSection] = useState<Section>("appearance");
  const [stats, setStats] = useState<Stats | null>(null);
  const [paths, setPaths] = useState<Paths | null>(null);
  const [build, setBuild] = useState<Build | null>(null);
  const [vault, setVault] = useState("");
  const [recent, setRecent] = useState<string[]>([]);
  const [onBattery, setOnBattery] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Всё сразу при открытии: экран небольшой, а подгружать разделы по одному
  // значит показывать «…» на каждом переключении.
  useEffect(() => {
    let cancelled = false;
    Promise.all([api.stats(), api.paths(), api.build(), api.onBattery()])
      .then(([gotStats, gotPaths, gotBuild, battery]) => {
        if (cancelled) return;
        setStats(gotStats);
        setPaths(gotPaths);
        setBuild(gotBuild);
        setOnBattery(battery);
        setVault(gotPaths.Vault);
      })
      .catch((err) => !cancelled && setError(describeError(err)));
    return () => {
      cancelled = true;
    };
  }, []);

  const loadVaults = () => {
    api
      .recentVaults()
      .then(setRecent)
      .catch((err) => setError(describeError(err)));
  };
  useEffect(loadVaults, []);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="overlay" onMouseDown={onClose}>
      <div className="prefs" onMouseDown={(event) => event.stopPropagation()}>
        <nav className="prefs__nav">
          <div className="prefs__navhead">Настройки</div>
          {sections.map((item) => (
            <button
              key={item.id}
              className="prefs__navitem"
              aria-selected={section === item.id}
              onClick={() => setSection(item.id)}
            >
              {item.label}
            </button>
          ))}
        </nav>

        <div className="prefs__body">
          <div className="prefs__title">
            {sections.find((item) => item.id === section)?.label}
            <button className="prefs__close" aria-label="закрыть" onClick={onClose}>
              ×
            </button>
          </div>

          <div className="prefs__content">
            {error && <div className="error">{error}</div>}

            {section === "appearance" && (
              <Appearance settings={settings} onChange={onChange} onBattery={onBattery} />
            )}
            {section === "storage" && (
              <Storage
                settings={settings}
                onChange={onChange}
                vault={vault}
                recent={recent}
                stats={stats}
                onStats={setStats}
                onVaults={loadVaults}
                onError={setError}
              />
            )}
            {section === "editor" && <EditorPrefs settings={settings} onChange={onChange} />}
            {section === "shortcuts" && (
              <Shortcuts
                keymap={keymap}
                vimNavigation={settings.vimNavigation}
                onSave={onKeymap}
                onReset={onResetKeymap}
                path={paths?.Keymap ?? ""}
                onReveal={() => paths && void api.revealPath(paths.Keymap)}
              />
            )}
            {section === "agent" && (
              <Agent
                settings={settings}
                onChange={onChange}
                paths={paths}
                stats={stats}
                vault={vault}
              />
            )}
            {section === "about" && <About build={build} paths={paths} />}
          </div>
        </div>
      </div>
    </div>
  );
}
