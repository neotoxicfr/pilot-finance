"use client"
import * as React from "react"
import { Moon, Sun, Monitor } from "lucide-react"
import { useTheme } from "next-themes"
export function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const [mounted, setMounted] = React.useState(false)
  React.useEffect(() => {
    setMounted(true)
  }, [])
  if (!mounted) return <div className="p-2 w-9 h-9" />
  const toggleTheme = () => {
    if (theme === "system") setTheme("light")
    else if (theme === "light") setTheme("dark")
    else setTheme("system")
  }
  return (
    <button
      onClick={toggleTheme}
      className="p-2 text-slate-400 hover:text-blue-400 hover:bg-blue-500/10 rounded-full transition-all flex items-center justify-center"
      title={`Thème actuel : ${theme}`}
    >
      {theme === "light" && <Sun size={20} />}
      {theme === "dark" && <Moon size={20} />}
      {theme === "system" && <Monitor size={20} />}
    </button>
  )
}