"use client";

import {
  DndContext,
  closestCenter,
  type CollisionDetection,
  type DragEndEvent,
  PointerSensor,
  useDroppable,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  horizontalListSortingStrategy,
  verticalListSortingStrategy,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical } from "lucide-react";
import clsx from "clsx";
import { useSyncExternalStore, type ReactNode } from "react";

type Column = {
  id: string;
  label: string;
};

type Props<T> = {
  columns: Column[];
  itemsByColumn: Record<string, T[]>;
  getItemId: (item: T) => string;
  renderCard: (item: T) => ReactNode;
  cardClassName?: (item: T) => string | undefined;
  onDrop: (itemId: string, columnId: string) => void;
  onReorderColumns?: (orderedColumnIds: string[]) => void;
  // M8追加：カードドロップ後の、ドロップ先列の最終的なカード順（列を跨ぐ移動・
  // 同一列内の並び替えいずれの場合も呼ばれる）。指定した画面のみカードが
  // 並び替え可能になる（マイタスクは対象外、プロジェクトのカンバンのみ渡す想定）。
  onReorderCards?: (columnId: string, orderedItemIds: string[]) => void;
  renderColumnFooter?: (columnId: string) => ReactNode;
  renderColumnHeaderExtra?: (columnId: string) => ReactNode;
  renderColumnLabel?: (columnId: string, label: string) => ReactNode;
  trailingColumn?: ReactNode;
};

// ドラッグ中のアイテム種別（カード/列）ごとにドロップ先候補を絞り込む。
// 列の並び替え中にカード用のドロップ領域と衝突判定が混ざるのを防ぐ。
// スマホ幅（Tailwindのsm=640px未満）ではカンバンを横並びではなく縦積みで表示する
// （実機でのD&D操作性・横スクロールの分かりにくさ改善のため）。列の並び替え時の
// ドラッグアニメーションもレイアウトに合わせて縦/横を切り替える必要があるため
// （dnd-kitのSortableStrategyはCSSではなくJS側で決まる）、matchMediaで判定する。
const MOBILE_QUERY = "(max-width: 639px)";
function subscribeIsMobile(callback: () => void) {
  const mql = window.matchMedia(MOBILE_QUERY);
  mql.addEventListener("change", callback);
  return () => mql.removeEventListener("change", callback);
}
function getIsMobileSnapshot() {
  return window.matchMedia(MOBILE_QUERY).matches;
}
function useIsMobile(): boolean {
  return useSyncExternalStore(subscribeIsMobile, getIsMobileSnapshot, () => false);
}

const collisionDetection: CollisionDetection = (args) => {
  const isColumnDrag = args.active.data.current?.type === "column";
  const filtered = args.droppableContainers.filter((c) =>
    isColumnDrag
      ? c.data.current?.type === "column"
      : c.data.current?.type === "cardzone" || c.data.current?.type === "card",
  );
  return closestCenter({ ...args, droppableContainers: filtered });
};

// @dnd-kitを使った汎用D&Dボード。プロジェクトKanban（列=ProjectStatusColumn、
// ドロップでstatus_column_idを更新、列自体もD&Dで並び替え可能。M8で同一列内の
// カード並び替えにも対応）とマイタスク（列=固定4ステータス、ドロップでstatusを
// 更新、列並び替え・カード並び替えなし）の両方で共用する。列・カードの中身は
// 呼び出し側に委譲する。
export default function KanbanBoard<T>({
  columns,
  itemsByColumn,
  getItemId,
  renderCard,
  cardClassName,
  onDrop,
  onReorderColumns,
  onReorderCards,
  renderColumnFooter,
  renderColumnHeaderExtra,
  renderColumnLabel,
  trailingColumn,
}: Props<T>) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));
  const isMobile = useIsMobile();

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (!over) return;

    if (active.data.current?.type === "column") {
      if (!onReorderColumns) return;
      const activeId = String(active.id).replace(/^col:/, "");
      const overId = String(over.id).replace(/^col:/, "");
      if (activeId === overId) return;
      const oldIndex = columns.findIndex((c) => c.id === activeId);
      const newIndex = columns.findIndex((c) => c.id === overId);
      if (oldIndex === -1 || newIndex === -1) return;
      onReorderColumns(arrayMove(columns, oldIndex, newIndex).map((c) => c.id));
      return;
    }

    // カードのドロップ。overは他カードのid（重ねた場合）か、列自体のcardzone id
    // （空き領域にドロップした場合）のいずれか。
    const activeId = String(active.id);
    const overId = String(over.id);

    const overIsColumn = columns.some((c) => c.id === overId);
    const destColumnId = overIsColumn
      ? overId
      : columns.find((c) => (itemsByColumn[c.id] ?? []).some((item) => getItemId(item) === overId))?.id;
    if (!destColumnId) return;

    onDrop(activeId, destColumnId);

    if (onReorderCards) {
      const destItemIds = (itemsByColumn[destColumnId] ?? [])
        .map(getItemId)
        .filter((id) => id !== activeId);
      const insertAt = overIsColumn ? destItemIds.length : destItemIds.indexOf(overId);
      const newOrder = [
        ...destItemIds.slice(0, insertAt === -1 ? destItemIds.length : insertAt),
        activeId,
        ...destItemIds.slice(insertAt === -1 ? destItemIds.length : insertAt),
      ];
      onReorderCards(destColumnId, newOrder);
    }
  }

  return (
    <DndContext sensors={sensors} collisionDetection={collisionDetection} onDragEnd={handleDragEnd}>
      <div className="flex flex-col gap-4 pb-4 sm:flex-row sm:overflow-x-auto">
        <SortableContext
          items={columns.map((c) => `col:${c.id}`)}
          strategy={isMobile ? verticalListSortingStrategy : horizontalListSortingStrategy}
        >
          {columns.map((col) => (
            <KanbanColumn
              key={col.id}
              column={col}
              sortable={!!onReorderColumns}
              headerExtra={renderColumnHeaderExtra?.(col.id)}
              label={renderColumnLabel?.(col.id, col.label)}
            >
              <SortableContext
                items={(itemsByColumn[col.id] ?? []).map(getItemId)}
                strategy={verticalListSortingStrategy}
              >
                {(itemsByColumn[col.id] ?? []).map((item) => (
                  <KanbanCard key={getItemId(item)} id={getItemId(item)} className={cardClassName?.(item)}>
                    {renderCard(item)}
                  </KanbanCard>
                ))}
              </SortableContext>
              {renderColumnFooter?.(col.id)}
            </KanbanColumn>
          ))}
        </SortableContext>
        {trailingColumn}
      </div>
    </DndContext>
  );
}

function KanbanColumn({
  column,
  sortable,
  headerExtra,
  label,
  children,
}: {
  column: Column;
  sortable: boolean;
  headerExtra?: ReactNode;
  label?: ReactNode;
  children: ReactNode;
}) {
  const {
    setNodeRef: setSortableRef,
    attributes,
    listeners,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: `col:${column.id}`, data: { type: "column" }, disabled: !sortable });
  const { setNodeRef: setDroppableRef, isOver } = useDroppable({
    id: column.id,
    data: { type: "cardzone" },
  });

  const style = {
    transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
    transition,
  };

  return (
    <div
      ref={setSortableRef}
      style={style}
      className={clsx(
        "flex w-full shrink-0 flex-col gap-3 rounded-xl border bg-surface-muted/60 p-3 transition-colors sm:w-72",
        isOver ? "border-indigo-400 ring-2 ring-indigo-400/30" : "border-border",
        isDragging && "opacity-50",
      )}
    >
      <div className="flex items-center gap-1.5 px-0.5">
        {sortable && (
          <span
            {...attributes}
            {...listeners}
            className="touch-none cursor-grab select-none text-muted-foreground/60 hover:text-muted-foreground"
            aria-label="列をドラッグして並び替え"
          >
            <GripVertical className="h-3.5 w-3.5" />
          </span>
        )}
        {label ?? <span className="flex-1 text-sm font-semibold text-foreground">{column.label}</span>}
        {headerExtra}
      </div>
      <ul ref={setDroppableRef} className="flex min-h-2 flex-col gap-2">
        {children}
      </ul>
    </div>
  );
}

// ドラッグリスナーはカード全体ではなく専用ハンドル（⠿）にのみ付ける。
// カード全体に付けると、内部のボタン（タスク展開等）の最初のクリックを
// dnd-kitのPointerSensorが取りこぼす現象があったため（実機確認済み）。
// M8：useDraggableからuseSortableに変更し、同一列内でのD&D並び替えにも対応した
// （data.typeは引き続き"card"のまま、collisionDetectionでの絞り込みに使う）。
// touch-none（touch-action: none）はスマホでの必須設定。無いとブラウザ標準の
// スクロールジェスチャーがPointerSensorのドラッグ検知より先に発火し、
// タッチ操作でドラッグを開始できなくなる。opacity-0はgroup-hoverが効かない
// タッチ端末では永久に非表示になってしまうため、smブレークポイント未満
// （タッチ操作が前提のスマホ幅）では常時表示にしている。
function KanbanCard({
  id,
  children,
  className,
}: {
  id: string;
  children: ReactNode;
  className?: string;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
    data: { type: "card" },
  });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };
  return (
    <li
      ref={setNodeRef}
      style={style}
      className={clsx(
        "group flex items-start gap-1 rounded-lg border border-border bg-surface p-2.5 text-sm shadow-sm transition-shadow hover:shadow-md",
        isDragging && "opacity-50",
        className,
      )}
    >
      <span
        {...listeners}
        {...attributes}
        className="mt-0.5 touch-none cursor-grab select-none text-muted-foreground/40 opacity-100 transition-opacity hover:text-muted-foreground sm:opacity-0 sm:group-hover:opacity-100"
        aria-label="ドラッグして移動"
      >
        <GripVertical className="h-3.5 w-3.5" />
      </span>
      <div className="min-w-0 flex-1">{children}</div>
    </li>
  );
}
