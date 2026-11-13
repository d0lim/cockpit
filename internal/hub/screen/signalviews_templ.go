package screen

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

import (
	"slices"
	"sort"
	"strconv"

	"github.com/d0lim/cockpit/internal/contract"
)

func sortedKeys(m contract.Attrs) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func pct(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

func depthClass(d int) string { return "depth-" + itoa(min(d, 8)) }

func pickTrace(traces []traceSummary, id string) string {
	for _, t := range traces {
		if t.id == id {
			return id
		}
	}
	if len(traces) > 0 {
		return traces[0].id
	}
	return ""
}

func traceView(st *state, snap signalSnapshot, all []contract.Signal) templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var1 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var1 == nil {
			templ_7745c5c3_Var1 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		if traces := traceList(all); true {
			templ_7745c5c3_Err = emptyNotice(st, snap, len(traces)).Render(ctx, templ_7745c5c3_Buffer)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			if len(traces) > 0 {
				if picked := pickTrace(traces, st.query.Get("trace")); true {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 1, "<div class=\"card overflow-x-auto p-0\"><table class=\"grid-table\"><thead><tr><th>Root</th><th>Start</th><th>Length</th><th>Status</th><th>Spans</th></tr></thead> <tbody>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					if listed, hidden := listedTraces(traces, picked); true {
						for _, t := range listed {
							var templ_7745c5c3_Var2 = []any{templ.KV("row-on", t.id == picked)}
							templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var2...)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 2, "<tr class=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var3 string
							templ_7745c5c3_Var3, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var2).String())
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var3)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 3, "\"><td><a class=\"link\" href=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var4 templ.SafeURL
							templ_7745c5c3_Var4, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.href(map[string][]string{"trace": {t.id}, "span": nil})))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 49, Col: 107}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var4))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 4, "\">")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var5 string
							templ_7745c5c3_Var5, templ_7745c5c3_Err = templ.JoinStringErrs(t.root)
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 49, Col: 118}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var5))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 5, "</a> ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							if t.exec {
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 6, "<span class=\"text-xs text-muted\">exec</span> ")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
							}
							if t.service {
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 7, "<span class=\"text-xs text-muted\">service</span>")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 8, "</td><td>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var6 string
							templ_7745c5c3_Var6, templ_7745c5c3_Err = templ.JoinStringErrs(stamp(t.start))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 57, Col: 30}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var6))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 9, "</td><td>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var7 string
							templ_7745c5c3_Var7, templ_7745c5c3_Err = templ.JoinStringErrs(durationText(t.duration))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 58, Col: 40}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var7))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 10, "</td><td>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = badge(spanTone(t.status), string(t.status)).Render(ctx, templ_7745c5c3_Buffer)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 11, "</td><td>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var8 string
							templ_7745c5c3_Var8, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(t.spans))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 62, Col: 29}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var8))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 12, "</td></tr>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 13, " ")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						if hidden > 0 {
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 14, "<tr><td colspan=\"5\" class=\"text-sm text-muted\">")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var9 string
							templ_7745c5c3_Var9, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(hidden))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 66, Col: 70}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var9))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 15, " older service traces are not listed</td></tr>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 16, "</tbody></table></div>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = spanList(st, snap, picked, all).Render(ctx, templ_7745c5c3_Buffer)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
			}
		}
		return nil
	})
}

func spanList(st *state, snap signalSnapshot, trace string, all []contract.Signal) templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var10 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var10 == nil {
			templ_7745c5c3_Var10 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		if byTrace, _ := spansByTrace(all); true {
			if rows := spanRows(byTrace[trace]); true {
				var templ_7745c5c3_Var11 = []any{"grid gap-4", templ.KV("lg:grid-cols-[1fr_22rem]", pickSpan(rows, st.query.Get("span")) != nil)}
				templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var11...)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 17, "<div class=\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var12 string
				templ_7745c5c3_Var12, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var11).String())
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var12)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 18, "\"><ol class=\"card divide-y divide-line p-0\" aria-label=\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var13 string
				templ_7745c5c3_Var13, templ_7745c5c3_Err = templ.ResolveAttributeValue("spans of " + trace)
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 82, Col: 78}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var13)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 19, "\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				for _, r := range rows {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 20, "<li>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var14 = []any{"span-row", templ.KV("event-on", r.id == st.query.Get("span"))}
					templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var14...)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 21, "<a href=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var15 templ.SafeURL
					templ_7745c5c3_Var15, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.href(map[string][]string{"trace": {trace}, "span": {r.id}})))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 85, Col: 94}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var15))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 22, "\" class=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var16 string
					templ_7745c5c3_Var16, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var14).String())
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var16)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 23, "\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var17 = []any{"truncate text-sm", depthClass(r.depth)}
					templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var17...)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 24, "<span class=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var18 string
					templ_7745c5c3_Var18, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var17).String())
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var18)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 25, "\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var19 string
					templ_7745c5c3_Var19, templ_7745c5c3_Err = templ.JoinStringErrs(r.s.Span.Name)
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 86, Col: 79}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var19))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 26, "</span> <svg class=\"span-track\" viewBox=\"0 0 100 6\" preserveAspectRatio=\"none\" aria-hidden=\"true\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var20 = []any{"span-bar", "tone-" + string(spanTone(r.s.Span.Status)), templ.KV("span-marker", r.marker)}
					templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var20...)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 27, "<rect x=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var21 string
					templ_7745c5c3_Var21, templ_7745c5c3_Err = templ.ResolveAttributeValue(pct(r.offset))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 88, Col: 32}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var21)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 28, "\" y=\"0\" width=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var22 string
					templ_7745c5c3_Var22, templ_7745c5c3_Err = templ.ResolveAttributeValue(pct(r.width))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 88, Col: 61}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var22)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 29, "\" height=\"6\" class=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var23 string
					templ_7745c5c3_Var23, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var20).String())
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var23)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 30, "\"></rect></svg> <span class=\"text-xs text-muted\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var24 string
					templ_7745c5c3_Var24, templ_7745c5c3_Err = templ.JoinStringErrs(durationText(r.duration))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 91, Col: 35}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var24))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 31, " · ")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var25 string
					templ_7745c5c3_Var25, templ_7745c5c3_Err = templ.JoinStringErrs(string(r.s.Span.Status))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 91, Col: 66}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var25))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 32, " ")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					if r.sourceMark != "" {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 33, "· time ")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var26 string
						templ_7745c5c3_Var26, templ_7745c5c3_Err = templ.JoinStringErrs(r.sourceMark)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 93, Col: 32}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var26))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 34, " ")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					if p := placement(r.s.Seq, snap); p != "" {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 35, "· ")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var27 string
						templ_7745c5c3_Var27, templ_7745c5c3_Err = templ.JoinStringErrs(p)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 96, Col: 16}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var27))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 36, "</span></a></li>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 37, "</ol>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				if r := pickSpan(rows, st.query.Get("span")); r != nil {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 38, "<aside class=\"card text-sm\" aria-label=\"Span detail\"><dl class=\"kv\"><dt>span</dt><dd class=\"break-all\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var28 string
					templ_7745c5c3_Var28, templ_7745c5c3_Err = templ.JoinStringErrs(r.id)
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 107, Col: 35}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var28))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 39, "</dd><dt>start</dt><dd>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var29 string
					templ_7745c5c3_Var29, templ_7745c5c3_Err = templ.JoinStringErrs(r.s.Span.Start)
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 109, Col: 27}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var29))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 40, "</dd><dt>end</dt><dd>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var30 string
					templ_7745c5c3_Var30, templ_7745c5c3_Err = templ.JoinStringErrs(orElse(r.s.Span.End, "not ended"))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 111, Col: 46}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var30))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 41, "</dd><dt>status</dt><dd>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var31 string
					templ_7745c5c3_Var31, templ_7745c5c3_Err = templ.JoinStringErrs(string(r.s.Span.Status))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 113, Col: 36}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var31))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 42, "</dd><dt>time source</dt><dd>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var32 string
					templ_7745c5c3_Var32, templ_7745c5c3_Err = templ.JoinStringErrs(string(r.s.TSSource))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 115, Col: 33}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var32))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 43, "</dd>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					for _, k := range sortedKeys(r.s.Attrs) {
						if !isRef(k) {
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 44, "<dt>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var33 string
							templ_7745c5c3_Var33, templ_7745c5c3_Err = templ.JoinStringErrs(k)
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 118, Col: 16}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var33))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 45, "</dt><dd class=\"break-all\">")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var34 string
							templ_7745c5c3_Var34, templ_7745c5c3_Err = templ.JoinStringErrs(attrText(r.s.Attrs[k]))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 119, Col: 55}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var34))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 46, "</dd>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 47, "</dl><div class=\"mt-3 space-y-3\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = signalDetail(st, r.s).Render(ctx, templ_7745c5c3_Buffer)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 48, "</div>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					for _, l := range r.s.Span.Links {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 49, "<a class=\"link mt-2 block text-xs\" href=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var35 templ.SafeURL
						templ_7745c5c3_Var35, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.href(map[string][]string{"trace": {l.TraceID}, "span": nil})))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 127, Col: 127}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var35))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 50, "\">Follow the link to ")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var36 string
						templ_7745c5c3_Var36, templ_7745c5c3_Err = templ.JoinStringErrs(l.TraceID)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 127, Col: 160}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var36))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 51, "</a>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 52, "</aside>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 53, "</div>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
			}
		}
		return nil
	})
}

func pickSpan(rows []spanRow, id string) *spanRow {
	for i := range rows {
		if id != "" && rows[i].id == id {
			return &rows[i]
		}
	}
	return nil
}

func tableView(st *state, snap signalSnapshot, all []contract.Signal) templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var37 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var37 == nil {
			templ_7745c5c3_Var37 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		if list := st.markSteps(tables(all)); true {
			templ_7745c5c3_Err = emptyNotice(st, snap, len(list)).Render(ctx, templ_7745c5c3_Buffer)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			for _, t := range list {
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 54, "<section class=\"space-y-2\" aria-label=\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var38 string
				templ_7745c5c3_Var38, templ_7745c5c3_Err = templ.ResolveAttributeValue("table " + t.name)
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 149, Col: 60}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var38)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 55, "\"><h3 class=\"text-sm font-semibold\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var39 string
				templ_7745c5c3_Var39, templ_7745c5c3_Err = templ.JoinStringErrs(t.name)
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 150, Col: 46}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var39))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 56, " <span class=\"font-normal text-muted\">latest ")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var40 string
				templ_7745c5c3_Var40, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(len(t.rows)))
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 150, Col: 112}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var40))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 57, " of ")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var41 string
				templ_7745c5c3_Var41, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(t.total))
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 150, Col: 133}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var41))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 58, " rows</span></h3><div class=\"flex flex-wrap gap-1\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				for _, c := range t.columns {
					var templ_7745c5c3_Var42 = []any{"pill", templ.KV("pill-on", !slices.Contains(st.query["hide"], t.name+"."+c))}
					templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var42...)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 59, "<a href=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var43 templ.SafeURL
					templ_7745c5c3_Var43, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.toggle("hide", t.name+"."+c)))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 153, Col: 62}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var43))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 60, "\" class=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var44 string
					templ_7745c5c3_Var44, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var42).String())
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var44)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 61, "\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var45 string
					templ_7745c5c3_Var45, templ_7745c5c3_Err = templ.JoinStringErrs(c)
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 153, Col: 156}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var45))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 62, "</a>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 63, "</div><div class=\"card overflow-x-auto p-0\"><table class=\"grid-table\"><thead><tr><th>time</th>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				if t.steps {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 64, "<th>step</th>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				for _, c := range visible(st, t) {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 65, "<th>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var46 string
					templ_7745c5c3_Var46, templ_7745c5c3_Err = templ.JoinStringErrs(c)
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 165, Col: 16}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var46))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 66, "</th>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 67, "<th>bundle</th></tr></thead> <tbody>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				for i, s := range t.rows {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 68, "<tr><td class=\"font-mono text-xs\"><a class=\"link\" href=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var47 templ.SafeURL
					templ_7745c5c3_Var47, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.signalHref(s)))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 173, Col: 93}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var47))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 69, "\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var48 string
					templ_7745c5c3_Var48, templ_7745c5c3_Err = templ.JoinStringErrs(clock(s.TS))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 173, Col: 109}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var48))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 70, "</a></td>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					if t.steps {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 71, "<td>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = stepLinks(st, st.correlation().stepsOf(s)).Render(ctx, templ_7745c5c3_Buffer)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 72, "</td>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					for _, c := range visible(st, t) {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 73, "<td>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var49 string
						templ_7745c5c3_Var49, templ_7745c5c3_Err = templ.JoinStringErrs(cellOf(s, t.bodies[i], c))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 180, Col: 41}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var49))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 74, "</td>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 75, "<td class=\"text-xs text-muted\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var50 string
					templ_7745c5c3_Var50, templ_7745c5c3_Err = templ.JoinStringErrs(placement(s.Seq, snap))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 182, Col: 64}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var50))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 76, "</td></tr>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 77, "</tbody></table></div></section>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
			}
		}
		return nil
	})
}

func visible(st *state, t signalTable) []string {
	var out []string
	for _, c := range t.columns {
		if !slices.Contains(st.query["hide"], t.name+"."+c) {
			out = append(out, c)
		}
	}
	return out
}

func pickSource(sources []logSource, name string) *logSource {
	for i := range sources {
		if sources[i].name == name {
			return &sources[i]
		}
	}
	if len(sources) > 0 {
		return &sources[0]
	}
	return nil
}

func logsView(st *state, snap signalSnapshot) templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var51 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var51 == nil {
			templ_7745c5c3_Var51 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		if sources := logSources(snap.signals); true {
			templ_7745c5c3_Err = emptyNotice(st, snap, len(sources)).Render(ctx, templ_7745c5c3_Buffer)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			if src := pickSource(sources, st.query.Get("source")); src != nil {
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 78, "<div class=\"flex flex-wrap gap-1\" aria-label=\"Log sources\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				for _, s := range sources {
					var templ_7745c5c3_Var52 = []any{"pill", templ.KV("pill-on", s.name == src.name)}
					templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var52...)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 79, "<a href=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var53 templ.SafeURL
					templ_7745c5c3_Var53, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.href(map[string][]string{"source": {s.name}})))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 221, Col: 78}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var53))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 80, "\" class=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var54 string
					templ_7745c5c3_Var54, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var52).String())
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var54)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 81, "\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var55 string
					templ_7745c5c3_Var55, templ_7745c5c3_Err = templ.JoinStringErrs(s.name)
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 221, Col: 147}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var55))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 82, " · ")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var56 string
					templ_7745c5c3_Var56, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(len(s.lines)))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 221, Col: 173}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var56))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 83, "</a>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 84, "</div><div class=\"console log-box\"><div>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				for _, l := range lastN(src.lines, logWindow) {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 85, "<div><span class=\"text-muted\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var57 string
					templ_7745c5c3_Var57, templ_7745c5c3_Err = templ.JoinStringErrs(clock(l.TS))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 227, Col: 49}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var57))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 86, "</span> ")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var58 string
					templ_7745c5c3_Var58, templ_7745c5c3_Err = templ.JoinStringErrs(logBody(l))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 227, Col: 71}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var58))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 87, "</div>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 88, "</div></div>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
			}
		}
		return nil
	})
}

func logBody(s contract.Signal) string {
	if s.Event == nil {
		return ""
	}
	return s.Event.Body
}

var _ = templruntime.GeneratedTemplate
