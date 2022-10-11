import { Routes } from '@/lib/constants'
import type { HeaderLink } from './Header.types'
import { CloudIcon } from '@heroicons/react/outline'

interface MenuItem {
  link: Routes
  text: string
}

export const LOGGED_IN_MENU_ITEMS: MenuItem[] = [
  {
    link: Routes.Home,
    text: 'Home',
  },
  {
    link: Routes.Cloud,
    text: 'Cloud',
  },
  {
    link: Routes.Explore,
    text: 'Services',
  },
  {
    link: Routes.UserUsage,
    text: 'Usage',
  },
  {
    link: Routes.UserBilling,
    text: 'Billing',
  },
  {
    link: Routes.UserKeys,
    text: 'Tokens',
  },
]

export const LOGGED_OUT_HEADER_LINKS: HeaderLink[] = [
  {
    link: Routes.About,
    text: 'About',
  },
  {
    link: 'https://blog.m3o.com',
    text: 'Blog',
    external: true,
  },
  {
    link: Routes.Explore,
    text: 'Explore',
  },
  {
    link: Routes.Pricing,
    text: 'Pricing',
  },
]

export const LOGGED_IN_HEADER_LINKS: HeaderLink[] = []
