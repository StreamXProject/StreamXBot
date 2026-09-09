import React from 'react'
import { Keyboard } from 'lucide-react'
import { Dialog, Button } from '@/components/md3'
import { useUiStore } from '@/stores/uiStore'
import { SHORTCUTS } from '@/hooks/useKeyboardShortcuts'

export const Kbd: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <kbd className="inline-flex items-center justify-center min-w-6 h-6 px-1.5 rounded-xs bg-surface-highest text-on-surface type-label-md font-mono border border-outline-variant">{children}</kbd>
)

export const ShortcutsDialog: React.FC = () => {
  const open = useUiStore((s) => s.shortcutsOpen)
  const setOpen = useUiStore((s) => s.setShortcutsOpen)
  const groups = ['Playback', 'View', 'Navigation'] as const
  return (
    <Dialog open={open} onClose={() => setOpen(false)} title="Keyboard shortcuts" icon={<Keyboard />} size="lg" actions={<Button variant="text" onClick={() => setOpen(false)}>Done</Button>}>
      <div className="grid sm:grid-cols-2 gap-x-8 gap-y-6">
        {groups.map((g) => (
          <div key={g} className={g === 'Playback' ? 'sm:row-span-2' : ''}>
            <h3 className="type-label-lg text-primary mb-2">{g}</h3>
            <ul className="space-y-2">
              {SHORTCUTS.filter((s) => s.group === g).map((s) => (
                <li key={s.description} className="flex items-center justify-between gap-4">
                  <span className="type-body-md text-on-surface">{s.description}</span>
                  <span className="flex items-center gap-1 shrink-0">
                    {s.keys.map((k, i) => (
                      <React.Fragment key={k}>
                        {i > 0 && <span className="text-on-surface-variant text-[10px]">+</span>}
                        <Kbd>{k}</Kbd>
                      </React.Fragment>
                    ))}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </Dialog>
  )
}
