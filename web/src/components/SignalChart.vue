<script setup lang="ts">
import {computed,ref,onMounted,onBeforeUnmount} from 'vue'
interface Point {x:number;label:string;values:Record<string,number|null>}
interface Series {key:string;label:string;color:string}
const props=defineProps<{id:string;label:string;points:Point[];series:Series[]}>()
const selected=ref<number|null>(null),element=ref<HTMLElement|null>(null),width=ref(760),left=46,top=18,bottom=174
const right=computed(()=>width.value-16)
let observer:ResizeObserver|undefined
onMounted(()=>{const resize=()=>{width.value=Math.max(280,Math.round(element.value?.clientWidth||760))};resize();if(typeof ResizeObserver!=='undefined'&&element.value){observer=new ResizeObserver(resize);observer.observe(element.value)}})
onBeforeUnmount(()=>observer?.disconnect())
const maximum=computed(()=>Math.max(1,...props.points.flatMap(p=>props.series.map(s=>p.values[s.key]??0))))
const ticks=computed(()=>maximum.value>1?[0,.5,1]:[0,1])
const active=computed(()=>selected.value===null||!props.points.length?null:Math.min(selected.value,props.points.length-1))
function x(i:number){if(props.points.length<2)return (left+right.value)/2;const start=props.points[0]!.x,end=props.points.at(-1)!.x;return left+(props.points[i]!.x-start)/(end-start||1)*(right.value-left)}
function y(value:number){return bottom-value/maximum.value*(bottom-top)}
function line(key:string){let d='',open=false;props.points.forEach((p,i)=>{const v=p.values[key];if(v===null||v===undefined){open=false;return}d+=`${open?'L':'M'}${x(i)},${y(v)} `;open=true});return d}
function area(key:string){if(!props.points.length||props.points.some(p=>p.values[key]==null))return '';return `${line(key)} L${x(props.points.length-1)},${bottom} L${x(0)},${bottom} Z`}
function number(value:number|null|undefined){return value==null?'—':value.toLocaleString(undefined,{maximumFractionDigits:1})}
function inspect(event:PointerEvent){if(!props.points.length)return;const bounds=(event.currentTarget as SVGSVGElement).getBoundingClientRect();if(!bounds.width)return;const mouse=(event.clientX-bounds.left)/bounds.width*width.value;selected.value=props.points.reduce((best,_,i)=>Math.abs(x(i)-mouse)<Math.abs(x(best)-mouse)?i:best,0)}
function step(direction:number){if(props.points.length)selected.value=selected.value===null?props.points.length-1:Math.max(0,Math.min(props.points.length-1,selected.value+direction))}
</script>
<template>
 <div ref="element" class="signal-chart" :data-samples="points.length">
  <div class="chart-legend"><span v-for="s in series" :key="s.key"><i :style="{background:s.color}"/>{{s.label}} <strong>{{number(points.at(-1)?.values[s.key])}}</strong></span></div>
  <div class="plot-wrap">
   <svg :viewBox="`0 0 ${width} 205`" role="img" :aria-label="label+'; use left and right arrow keys to inspect samples'" tabindex="0" @pointermove="inspect" @pointerleave="selected=null" @focus="step(0)" @blur="selected=null" @keydown.left.prevent="step(-1)" @keydown.right.prevent="step(1)">
    <defs><linearGradient :id="id+'-fill'" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" :stop-color="series[0]?.color" stop-opacity=".18"/><stop offset="100%" :stop-color="series[0]?.color" stop-opacity="0"/></linearGradient></defs>
    <g v-for="tick in ticks" :key="tick"><line :x1="left" :x2="right" :y1="y(maximum*tick)" :y2="y(maximum*tick)" class="chart-grid"/><text :x="left-10" :y="y(maximum*tick)+4" text-anchor="end" class="chart-label">{{number(maximum*tick)}}</text></g>
    <path v-if="series[0]" :d="area(series[0].key)" :fill="`url(#${id}-fill)`"/>
    <g v-for="s in series" :key="s.key"><path :d="line(s.key)" fill="none" :stroke="s.color" stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke"/><circle v-if="points.length&&points.at(-1)?.values[s.key]!=null" :cx="x(points.length-1)" :cy="y(points.at(-1)!.values[s.key]!)" r="3.5" :fill="s.color"/></g>
    <template v-if="active!==null"><line :x1="x(active)" :x2="x(active)" :y1="top" :y2="bottom" class="chart-cursor"/><template v-for="s in series" :key="s.key"><circle v-if="points[active]!.values[s.key]!=null" :cx="x(active)" :cy="y(points[active]!.values[s.key]!)" r="4.5" :fill="s.color"/></template></template>
    <text :x="left" y="198" class="chart-label">{{points[0]?.label}}</text><text v-if="points.length>1" :x="right" y="198" text-anchor="end" class="chart-label">{{points.at(-1)?.label}}</text>
   </svg>
   <div v-if="active!==null" class="chart-tooltip"><time>{{points[active]!.label}}</time><span v-for="s in series" :key="s.key">{{s.label}} <strong>{{number(points[active]!.values[s.key])}}</strong></span></div>
   <span v-if="!points.length" class="chart-empty">No samples yet</span>
  </div>
 </div>
</template>
<style scoped>
.signal-chart{min-width:0}.chart-legend{display:flex;flex-wrap:wrap;gap:8px 20px;padding:0 4px 12px;font-size:11px;color:var(--chart-ink,var(--muted))}.chart-legend>span{display:flex;align-items:center;gap:6px}.chart-legend i{width:15px;height:3px;border-radius:3px}.chart-legend strong{font-variant-numeric:tabular-nums;font-size:12px;color:var(--chart-strong,var(--ink));margin-left:3px}.plot-wrap{position:relative}svg{display:block;width:100%;overflow:visible;touch-action:pan-y}.chart-grid{stroke:var(--chart-grid,var(--line));stroke-width:1;vector-effect:non-scaling-stroke}.chart-label{fill:var(--chart-ink,var(--muted));font:11px var(--mono)}.chart-cursor{stroke:var(--chart-ink,var(--muted));stroke-dasharray:3 4;opacity:.65;vector-effect:non-scaling-stroke}.chart-tooltip{position:absolute;right:12px;top:4px;pointer-events:none;display:grid;gap:5px;min-width:145px;max-width:calc(100% - 24px);padding:10px 13px;border:1px solid var(--chart-grid,var(--line));border-radius:7px;background:var(--chart-tooltip,var(--paper));box-shadow:0 5px 16px #00000018;color:var(--chart-strong,var(--ink));font-size:11px}.chart-tooltip time{color:var(--chart-ink,var(--muted));font-family:var(--mono)}.chart-tooltip span{display:flex;justify-content:space-between;gap:20px}.chart-tooltip strong{font-variant-numeric:tabular-nums}.chart-empty{position:absolute;inset:30% 0 auto;text-align:center;color:var(--chart-ink,var(--muted));font-size:12px}@media(max-width:600px){.chart-legend{gap:7px 12px}svg{min-height:155px}}
</style>
