import React from 'react'
import { cn } from '@/lib/cn'

export const PageContainer: React.FC<React.HTMLAttributes<HTMLDivElement> & { narrow?: boolean }> = ({ className, narrow, ...rest }) => (
  <div className={cn('page-enter px-4 sm:px-6 lg:px-8 pt-4 sm:pt-6 pb-10 mx-auto w-full', narrow ? 'max-w-4xl' : 'max-w-[1400px]', className)} {...rest} />
)
