import React, { useId, useRef, useImperativeHandle } from 'react'
import { cn } from '@/lib/cn'

export interface TextFieldProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'size'> {
  label?: string
  supporting?: string
  error?: string | null
  leading?: React.ReactNode
  trailing?: React.ReactNode
  variant?: 'outlined' | 'filled'
  containerClassName?: string
}

export const TextField = React.forwardRef<HTMLInputElement, TextFieldProps>(
  ({ label, supporting, error, leading, trailing, variant = 'outlined', className, containerClassName, id, placeholder, ...rest }, ref) => {
    const autoId = useId()
    const inputId = id ?? autoId
    const hasError = Boolean(error)
    const inputRef = useRef<HTMLInputElement>(null)

    useImperativeHandle(ref, () => inputRef.current as HTMLInputElement)

    const handleContainerClick = (e: React.MouseEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement
      if (target !== inputRef.current && !target.closest('button') && !target.closest('a')) {
        inputRef.current?.focus()
      }
    }

    return (
      <div className={cn('flex flex-col gap-1', containerClassName)}>
        <div
          onClick={handleContainerClick}
          className={cn(
            'relative flex items-center gap-3 h-14 px-4 transition-colors cursor-text',
            variant === 'outlined'
              ? cn('rounded-xs border', hasError ? 'border-error' : 'border-outline focus-within:border-primary focus-within:ring-1 focus-within:ring-primary')
              : cn('rounded-t-xs bg-surface-highest border-b', hasError ? 'border-error' : 'border-on-surface-variant focus-within:border-primary'),
            rest.disabled && 'opacity-40 cursor-not-allowed'
          )}
        >
          {leading && <span className="text-on-surface-variant shrink-0 [&_svg]:size-5 pointer-events-none">{leading}</span>}
          <div className="relative flex-1 h-full">
            <input
              ref={inputRef}
              id={inputId}
              placeholder={placeholder || (label ? ' ' : undefined)}
              className={cn(
                'peer w-full h-full bg-transparent outline-none text-on-surface type-body-lg',
                label
                  ? 'pt-3 placeholder:opacity-0 focus:placeholder:opacity-100 placeholder:text-on-surface-variant/60 placeholder:transition-opacity'
                  : 'placeholder:text-on-surface-variant/60',
                className
              )}
              aria-invalid={hasError}
              {...rest}
            />
            {label && (
              <label
                htmlFor={inputId}
                className={cn(
                  'absolute left-0 pointer-events-none transition-all duration-150 ease-standard',
                  'top-1/2 -translate-y-1/2 type-body-lg text-on-surface-variant',
                  'peer-focus:top-2 peer-focus:translate-y-0 peer-focus:text-[12px] peer-focus:leading-4',
                  'peer-[:not(:placeholder-shown)]:top-2 peer-[:not(:placeholder-shown)]:translate-y-0 peer-[:not(:placeholder-shown)]:text-[12px] peer-[:not(:placeholder-shown)]:leading-4',
                  hasError ? 'text-error' : 'peer-focus:text-primary'
                )}
              >
                {label}
              </label>
            )}
          </div>
          {trailing && <span className="text-on-surface-variant shrink-0 [&_svg]:size-5 flex items-center">{trailing}</span>}
        </div>
        {(error || supporting) && (
          <p className={cn('type-body-sm px-4', hasError ? 'text-error' : 'text-on-surface-variant')}>{error || supporting}</p>
        )}
      </div>
    )
  }
)
TextField.displayName = 'TextField'
