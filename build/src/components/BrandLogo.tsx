import React from 'react';
export default function BrandLogo({ size = 32, withText = true }: { size?: number, withText?: boolean }) {
  return (
    <div className="flex items-center gap-3 select-none">
      <svg 
        width={size} 
        height={size} 
        viewBox="0 0 500 500" 
        fill="none" 
        xmlns="http://www.w3.org/2000/svg"
      >
        {}
        <path 
            d="M476.8 143.5C490.2 172.4 498 203.6 499.7 235.5C501.7 274.9 494.4 314.3 478.4 350.3L432.7 330C445.5 301.1 451.4 269.7 449.7 238.2C449.1 227 447.6 216 445.2 205.1C463.7 197.8 476.8 179.7 476.8 158.6V143.5ZM276.1 0C314.9 4.1 352.2 17.2 385 38.2L334.5 67.4C314.5 58 293 52 270.9 49.7L276.1 0Z" 
            fill="url(#grad_green)"
        />
        {}
        <path 
            d="M452.3 395.6C429.1 427.5 398.7 453.5 363.5 471.4C328.3 489.3 289.5 498.6 250 498.6C210.5 498.6 171.7 489.3 136.5 471.4C128 467.1 119.8 462.3 111.9 457C113.6 455 115.1 452.8 116.5 450.4L137.5 414C144.5 418.7 151.7 423 159.2 426.8C187.3 441.2 218.4 448.6 250 448.6C281.6 448.6 312.7 441.2 340.8 426.8C368.9 412.5 393.3 391.7 411.8 366.2L452.3 395.6Z" 
            fill="url(#grad_violet)"
        />
        {}
        <path 
            d="M21.6 350.3C5.6 314.3 -1.7 274.9 0.3 235.5C2.4 196.1 13.8 157.8 33.5 123.6C53.2 89.5 80.8 60.5 113.8 39C146.9 17.5 184.6 4.1 223.9 0L229.1 49.7C197.7 53 167.5 63.7 141.1 80.9C114.6 98.1 92.6 121.3 76.8 148.6C61 176 51.9 206.6 50.3 238.2C48.6 269.7 54.5 301.1 67.3 330L21.6 350.3Z" 
            fill="url(#grad_blue)"
        />
        {}
        <path 
            d="M73.2 425.4L212.6 183.8L287.4 313.4L426.8 71.8M426.8 71.8L351.6 115.2M426.8 71.8L426.8 158.6" 
            stroke="url(#grad_green)" 
            strokeWidth="50" 
            strokeLinecap="round" 
            strokeLinejoin="round"
        />
        <defs>
            <linearGradient id="grad_blue" x1="517" y1="93" x2="-17" y2="403" gradientUnits="userSpaceOnUse">
                <stop stopColor="#2563EB"/>
                <stop offset="1" stopColor="#3B82F6"/>
            </linearGradient>
            <linearGradient id="grad_violet" x1="488" y1="112" x2="11" y2="384" gradientUnits="userSpaceOnUse">
                <stop stopColor="#C084FC"/>
                <stop offset="1" stopColor="#8B5CF6"/>
            </linearGradient>
            <linearGradient id="grad_green" x1="426" y1="93" x2="351" y2="136" gradientUnits="userSpaceOnUse">
                <stop stopColor="#10B981"/>
                <stop offset="1" stopColor="#34D399"/>
            </linearGradient>
        </defs>
      </svg>
      {withText && (
        <span className="text-xl font-bold tracking-tight text-foreground leading-none">
          Pilot
        </span>
      )}
    </div>
  );
}