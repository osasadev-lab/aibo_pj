"use client";

import {
  DndContext,
  closestCenter,
  type CollisionDetection,
  type DragEndEvent,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import { SortableContext, arrayMove, horizontalListSortingStrategy, useSortable } from "@dnd-kit/sortable";
import { GripVertical } from "lucide-react";
import clsx from "clsx";
import type { ReactNode } from "react";

type Column = {
  id: string;
  label: string;
};

type Props<T> = {
  columns: Column[];
  itemsByColumn: Record<string, T[]>;
  getItemId: (item: T) => string;
  renderCard: (item: T) => ReactNode;
  onDrop: (itemId: string, columnId: string) => void;
  onReorderColumns?: (orderedColumnIds: string[]) => void;
  renderColumnFooter?: (columnId: string) => ReactNode;
  renderColumnHeaderExtra?: (columnId: string) => ReactNode;
  renderColumnLabel?: (columnId: string, label: string) => ReactNode;
  trailingColumn?: ReactNode;
};

// ドラッグ中のアイテム種別（カード/列）ごとにドロップ先候補を絞り込む。
// 列の並び替え中にカード用のドロップ領域と衝突判定が混ざるのを防ぐ。
const collisionDetection: CollisionDetection = (args) => {
  const isColumnDrag = args.active.data.current?.type === "column";
  const filtered = args.droppableContainers.filter((c) =>
    isColumnDrag ? c.data.current?.type === "column" : c.data.current?.type === "cardzone",
  );
  return closestCenter({ ...args, droppableContainers: filtered });
};

// @dnd-kitを使った汎用D&Dボード。プロジェクトKanban（列=ProjectStatusColumn、
// ドロップでstatus_column_idを更新、列自体もD&Dで並び替え可能）とマイタスク
// （列=固定4ステータス、ドロップでstatusを更新、列並び替えなし）の両方で共用する。
// 列・カードの中身は呼び出し側に委譲する。
export default function KanbanBoard<T>({
  columns,
  itemsByColumn,
  getItemId,
  renderCard,
  onDrop,
  onReorderColumns,
  renderColumnFooter,
  renderColumnHeaderExtra,
  renderColumnLabel,
  trailingColumn,
}: Props<T>) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

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

    onDrop(String(active.id), String(over.id));
  }

  return (
    <DndContext sensors={sensors} collisionDetection={collisionDetection} onDragEnd={handleDragEnd}>
      <div className="flex gap-4 overflow-x-auto pb-4">
        <SortableContext
          items={columns.map((c) => `col:${c.id}`)}
          strategy={horizontalListSortingStrategy}
        >
          {columns.map((col) => (
            <KanbanColumn
              key={col.id}
              column={col}
              sortable={!!onReorderColumns}
              headerExtra={renderColumnHeaderExtra?.(col.id)}
              label={renderColumnLabel?.(col.id, col.label)}
            >
              {(itemsByColumn[col.id] ?? []).map((item) => (
                <KanbanCard key={getItemId(item)} id={getItemId(item)}>
                  {renderCard(item)}
                </KanbanCard>
              ))}
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
        "flex w-72 shrink-0 flex-col gap-3 rounded-xl border bg-surface-muted/60 p-3 transition-colors",
        isOver ? "border-indigo-400 ring-2 ring-indigo-400/30" : "border-border",
        isDragging && "opacity-50",
      )}
    >
      <div className="flex items-center gap-1.5 px-0.5">
        {sortable && (
          <span
            {...attributes}
            {...listeners}
            className="cursor-grab select-none text-muted-foreground/60 hover:text-muted-foreground"
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
function KanbanCard({ id, children }: { id: string; children: ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id,
    data: { type: "card" },
  });
  const style = transform
    ? { transform: `translate3d(${transform.x}px, ${transform.y}px, 0)` }
    : undefined;
  return (
    <li
      ref={setNodeRef}
      style={style}
      className={clsx(
        "group flex items-start gap-1 rounded-lg border border-border bg-surface p-2.5 text-sm shadow-sm transition-shadow hover:shadow-md",
        isDragging && "opacity-50",
      )}
    >
      <span
        {...listeners}
        {...attributes}
        className="mt-0.5 cursor-grab select-none text-muted-foreground/40 opacity-0 transition-opacity group-hover:opacity-100 hover:text-muted-foreground"
        aria-label="ドラッグして移動"
      >
        <GripVertical className="h-3.5 w-3.5" />
      </span>
      <div className="min-w-0 flex-1">{children}</div>
    </li>
  );
}
