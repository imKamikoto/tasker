/** Где стоит выделение: координаты окна, как их отдаёт CodeMirror. */
export type SelectionRect = { top: number; bottom: number; left: number };

/** Размеры коробки, внутри которой живёт тулбар, и его собственные. */
export type Box = { width: number; height: number };

/**
 * toolbarPosition считает, куда поставить плавающий тулбар.
 *
 * Чистой функцией и с тестами, потому что ошибки здесь не видны в коде и
 * заметны только глазами: тулбар, уехавший под верхнюю кромку окна или за
 * правый край, выглядит как «кнопки пропали».
 *
 * Правило: над выделением, если сверху есть место, иначе под ним. По
 * горизонтали прижимается к краям окна, но не выходит за них.
 */
export function toolbarPosition(
  /** Координаты выделения относительно коробки, а не окна. */
  selection: SelectionRect,
  /**
   * Коробка — колонка редактора, а не окно: у неё backdrop-filter, а он
   * создаёт содержащий блок для позиционированных потомков, и абсолютные
   * координаты отсчитываются от неё. Заодно тулбар не вылезает в соседнюю
   * колонку, чего окно бы не запретило.
   */
  container: Box,
  toolbar: Box,
  /** Верхняя граница, ниже которой можно рисовать: полоса перетаскивания окна. */
  safeTop = 42,
): { left: number; top: number } {
  const gap = 8;

  const above = selection.top - toolbar.height - gap;
  const top = above >= safeTop ? above : selection.bottom + gap;

  const edge = 8;
  const left = Math.min(
    Math.max(edge, selection.left - toolbar.width / 2),
    Math.max(edge, container.width - toolbar.width - edge),
  );
  return { left, top };
}
