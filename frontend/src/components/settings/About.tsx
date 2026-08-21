import { api, type Build, type Paths } from "../../api";
import { Card, Fact, PathLine, Row } from "./controls";

type Props = {
  build: Build | null;
  paths: Paths | null;
};

export function About({ build, paths }: Props) {
  const reveal = (path: string) => () => void api.revealPath(path);

  return (
    <>
      <Card>
        <Row label="Tasker" hint="Локальный редактор заметок-как-задач под macOS">
          <div className="facts">
            <Fact
              label="Сборка"
              value={
                build?.Revision
                  ? `${build.Revision.slice(0, 12)}${build.Modified ? " + правки" : ""}`
                  : "из исходников"
              }
            />
            <Fact label="Собрана" value={build?.Time ? build.Time.slice(0, 10) : "—"} />
            <Fact label="Go" value={build?.Go ?? "—"} />
            <Fact label="Wails" value={build?.Wails || "—"} />
          </div>
        </Row>
      </Card>

      <Card>
        <Row label="Где что лежит">
          <div className="stack">
            {paths &&
              (
                [
                  ["Заметки", paths.Vault],
                  ["Настройки", paths.Config],
                  ["Клавиши", paths.Keymap],
                  ["Хранилища", paths.Vaults],
                  ["Индекс", paths.Index],
                ] as const
              ).map(([label, path]) => (
                <div key={label} className="pathrow">
                  <span className="pathrow__label">{label}</span>
                  <PathLine path={path} onReveal={reveal(path)} />
                </div>
              ))}
          </div>
        </Row>
      </Card>

      <p className="section-note">
        Заметки — обычные <code>.md</code> файлы, индекс производный и восстанавливается из них.
        История в git рядом с заметками. Ничего из этого никуда не отправляется: ни аккаунтов, ни
        синхронизации, ни ключей моделей внутри приложения нет.
      </p>
    </>
  );
}
