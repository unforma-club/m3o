import type { AppProps } from 'next/app'
import type { AxiosError } from 'axios'
import { DefaultSeo } from 'next-seo'
import { Toaster, toast } from 'react-hot-toast'
import { ReactQueryDevtools } from 'react-query/devtools'
import { useState, useEffect } from 'react'
import { useRouter } from 'next/router'
import { ThemeProvider } from 'next-themes'
import { QueryClient, QueryClientProvider, MutationCache } from 'react-query'
import { Hydrate } from 'react-query/hydration'
import { CookiesProvider } from 'react-cookie'
import { UserProvider, ToastProvider } from '@/providers'
import { Tracker } from '@/components/ui'
import * as gtag from '@/lib/gtag'

import 'tailwindcss/tailwind.css'
import '../styles/globals.css'

function MyApp({ Component, pageProps }: AppProps) {
  const { user } = pageProps
  const router = useRouter()

  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            refetchOnWindowFocus: false,
            retry: false,
          },
        },
        mutationCache: new MutationCache({
          onError: error => {
            const e = error as AxiosError

            if ((e.response?.data as ApiError).detail) {
              toast.error(e.response?.data.detail)
            }
          },
        }),
      }),
  )

  useEffect(() => {
    router.events.on('routeChangeComplete', gtag.pageview)

    return () => {
      router.events.off('routeChangeComplete', gtag.pageview)
    }
  }, [router])

  return (
    <>
      <DefaultSeo
        titleTemplate="%s | M3O"
        openGraph={{
          type: 'website',
          url: 'https://m3o.com',
          site_name: 'M3O',
          images: [
            {
              url: 'https://m3o.com/og/open-graph.png',
              width: 1200,
              height: 630,
              alt: 'Micro Services',
            },
          ],
        }}
      />
      <UserProvider user={user}>
        <CookiesProvider>
          <ToastProvider>
            <QueryClientProvider client={queryClient}>
              <Hydrate state={pageProps.dehydratedState}>
                <Tracker>
                  <ThemeProvider
                    attribute="class"
                    enableSystem={false}
                    >
                    <Component {...pageProps} />
                  </ThemeProvider>
                </Tracker>
              </Hydrate>
              <ReactQueryDevtools initialIsOpen={false} />
            </QueryClientProvider>
            <Toaster
              toastOptions={{
                style: {
                  background: '#333',
                  color: '#fff',
                },
              }}
            />
          </ToastProvider>
        </CookiesProvider>
      </UserProvider>
    </>
  )
}

export default MyApp
