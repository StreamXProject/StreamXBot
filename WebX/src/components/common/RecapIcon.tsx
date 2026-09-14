import React from 'react'

export interface RecapIconProps extends React.SVGProps<SVGSVGElement> {
  className?: string
  strokeWidth?: number | string
  withBar?: boolean
}

/**
 * Modern circular replay/recap icon inspired by media recap illustrations.
 * Conforms to Lucide icon props and stroke-width conventions.
 */
export const RecapIcon: React.FC<RecapIconProps> = ({
  className = 'size-6',
  strokeWidth = 2,
  withBar = false,
  ...props
}) => {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden="true"
      {...props}
    >

      <path d="M16.5 18A8.5 8.5 0 1 1 20 10" />
      <polyline points="21.5 5.5 20 10 15.5 9.5" />

      {withBar ? (
        <>
          <line x1="7.5" y1="9" x2="7.5" y2="15" />
          <polygon points="10.5 9 16.5 12 10.5 15 10.5 9" />
        </>
      ) : (
        <polygon points="9.5 8.5 15.5 12 9.5 15.5 9.5 8.5" />
      )}
    </svg>
  )
}
