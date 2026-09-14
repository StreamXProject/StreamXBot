import React from 'react'
import { X } from 'lucide-react'
import { useUiStore } from '@/stores/uiStore'
import { IconButton } from '@/components/md3'
import { QueueList } from './QueueList'

/** Right-side sheet (always mounted, CSS transform in/out) */
export const QueueDrawer: React.FC = () => {
  const open = useUiStore((s) => s.queueDrawerOpen)
  const close = useUiStore((s) => s.closeQueueDrawer)
  return (
    <>
      <div className="scrim fixed inset-0 z-40 bg-scrim/40 md:bg-transparent md:pointer-events-none" data-open={open} onClick={close} />
      <aside
        className="sheet-right fixed top-0 right-0 z-40 w-full sm:w-[400px] elev-1 border-l border-outline-variant/60 flex flex-col shadow-md3-3 md:shadow-none bottom-[calc(var(--webx-mini-player-height)+var(--webx-nav-height)+env(safe-area-inset-bottom,0px))] md:bottom-[var(--webx-player-height)]"
        data-open={open}
        aria-hidden={!open}
        // @ts-expect-error
        inert={open ? undefined : ''}
      >
        <div className="flex items-center justify-between pl-5 pr-3 h-[calc(var(--webx-topbar-height)+env(safe-area-inset-top,0px))] pt-[env(safe-area-inset-top,0px)] shrink-0 border-b border-outline-variant/60">
          <h2 className="type-title-lg text-on-surface">Queue</h2>
          <IconButton label="Close queue" onClick={close}><X /></IconButton>
        </div>
        <QueueList className="flex-1" />
      </aside>
    </>
  )
}
