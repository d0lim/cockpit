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
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 1, "<div class=\"grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-[17rem_minmax(0,1fr)]\"><nav class=\"rows stick max-h-72\" aria-label=\"Traces\" data-scroll-box=\"traces\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					if listed, hidden := listedTraces(traces, picked); true {
						for _, t := range listed {
							var templ_7745c5c3_Var2 = []any{"trace-row", templ.KV("event-on", t.id == picked)}
							templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var2...)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 2, "<a class=\"")
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
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 3, "\" href=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var4 templ.SafeURL
							templ_7745c5c3_Var4, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.href(map[string][]string{"trace": {t.id}, "span": nil, "open": nil})))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 45, Col: 164}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var4))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 4, "\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							if t.id == picked {
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 5, " aria-current=\"true\" data-scroll-here=\"")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
								var templ_7745c5c3_Var5 string
								templ_7745c5c3_Var5, templ_7745c5c3_Err = templ.ResolveAttributeValue("trace-" + t.id)
								if templ_7745c5c3_Err != nil {
									return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 45, Col: 241}
								}
								_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var5)
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 6, "\"")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 7, "><span class=\"flex items-center gap-2\">")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var6 = []any{"dot", "tone-" + string(spanTone(t.status))}
							templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var6...)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 8, "<span class=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var7 string
							templ_7745c5c3_Var7, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var6).String())
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var7)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 9, "\" title=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var8 string
							templ_7745c5c3_Var8, templ_7745c5c3_Err = templ.ResolveAttributeValue(string(t.status))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 47, Col: 94}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var8)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 10, "\"></span> <span class=\"min-w-0 truncate text-[13px] font-medium\">")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var9 string
							templ_7745c5c3_Var9, templ_7745c5c3_Err = templ.JoinStringErrs(t.root)
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 48, Col: 73}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var9))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 11, "</span> ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							if t.exec {
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 12, "<span class=\"ml-auto text-[11px] text-muted\">exec</span> ")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
							}
							if t.service {
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 13, "<span class=\"ml-auto text-[11px] text-muted\">service</span>")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 14, "</span> <span class=\"pl-4 text-[11px] text-muted\">")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var10 string
							templ_7745c5c3_Var10, templ_7745c5c3_Err = templ.JoinStringErrs(stamp(t.start))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 56, Col: 67}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var10))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 15, " · ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var11 string
							templ_7745c5c3_Var11, templ_7745c5c3_Err = templ.JoinStringErrs(durationText(t.duration))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 56, Col: 99}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var11))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 16, " · ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var12 string
							templ_7745c5c3_Var12, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(t.spans))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 56, Col: 120}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var12))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 17, " spans · ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var13 string
							templ_7745c5c3_Var13, templ_7745c5c3_Err = templ.JoinStringErrs(string(t.status))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 56, Col: 150}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var13))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 18, "</span></a>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 19, " ")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						if hidden > 0 {
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 20, "<p class=\"px-3 py-2 text-xs text-muted\">")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var14 string
							templ_7745c5c3_Var14, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(hidden))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 60, Col: 62}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var14))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 21, " older service traces are not listed</p>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 22, "</nav>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = spanList(st, snap, picked, all).Render(ctx, templ_7745c5c3_Buffer)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 23, "</div>")
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
		templ_7745c5c3_Var15 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var15 == nil {
			templ_7745c5c3_Var15 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		if byTrace, _ := spansByTrace(all); true {
			if rows := spanRows(byTrace[trace]); true {
				if fold := st.foldSpans(rows); true {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 24, "<div class=\"min-w-0 space-y-2\"><p class=\"flex flex-wrap items-center gap-2 text-xs text-muted\"><span>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var16 string
					templ_7745c5c3_Var16, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(len(fold.rows)))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 77, Col: 34}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var16))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 25, " of ")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var17 string
					templ_7745c5c3_Var17, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(fold.total))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 77, Col: 58}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var17))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 26, " spans shown</span> ")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					if fold.all {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 27, "<a class=\"link\" href=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var18 templ.SafeURL
						templ_7745c5c3_Var18, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.spanAllHref(false)))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 79, Col: 66}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var18))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 28, "\" hx-swap=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var19 string
						templ_7745c5c3_Var19, templ_7745c5c3_Err = templ.ResolveAttributeValue(keepPlace)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 79, Col: 88}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var19)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 29, "\">Fold to four levels</a>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					} else if len(fold.rows) < fold.total {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 30, "<a class=\"link\" href=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var20 templ.SafeURL
						templ_7745c5c3_Var20, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.spanAllHref(true)))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 81, Col: 65}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var20))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 31, "\" hx-swap=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var21 string
						templ_7745c5c3_Var21, templ_7745c5c3_Err = templ.ResolveAttributeValue(keepPlace)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 81, Col: 87}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var21)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 32, "\">Open every span</a>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 33, "</p><div class=\"split\"><div class=\"split-grid\"><ol class=\"rows overflow-x-auto\" aria-label=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var22 string
					templ_7745c5c3_Var22, templ_7745c5c3_Err = templ.ResolveAttributeValue("spans of " + trace)
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 86, Col: 71}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var22)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 34, "\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					for _, r := range fold.rows {
						var templ_7745c5c3_Var23 = []any{"span-row", templ.KV("event-on", r.id == st.query.Get("span"))}
						templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var23...)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 35, "<li class=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var24 string
						templ_7745c5c3_Var24, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var23).String())
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var24)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 36, "\">")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var25 = []any{"span-name", depthClass(r.depth)}
						templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var25...)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 37, "<span class=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var26 string
						templ_7745c5c3_Var26, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var25).String())
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var26)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 38, "\">")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						if r.toggle != "" {
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 39, "<a class=\"span-toggle\" href=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var27 templ.SafeURL
							templ_7745c5c3_Var27, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(r.toggle))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 91, Col: 64}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var27))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 40, "\" hx-swap=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var28 string
							templ_7745c5c3_Var28, templ_7745c5c3_Err = templ.ResolveAttributeValue(keepPlace)
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 91, Col: 86}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var28)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 41, "\" title=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var29 string
							templ_7745c5c3_Var29, templ_7745c5c3_Err = templ.ResolveAttributeValue(foldTitle(r))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 91, Col: 109}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var29)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 42, "\" aria-label=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var30 string
							templ_7745c5c3_Var30, templ_7745c5c3_Err = templ.ResolveAttributeValue(foldTitle(r))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 91, Col: 137}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var30)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 43, "\" aria-expanded=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var31 string
							templ_7745c5c3_Var31, templ_7745c5c3_Err = templ.ResolveAttributeValue(boolText(r.open))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 91, Col: 172}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var31)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 44, "\">")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							if r.open {
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 45, "▼")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
							} else {
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 46, "▶")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 47, "</a> ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						} else {
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 48, "<span class=\"size-4 shrink-0\"></span> ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 49, "<a class=\"min-w-0 truncate hover:underline\" href=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var32 templ.SafeURL
						templ_7745c5c3_Var32, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.href(map[string][]string{"trace": {trace}, "span": {r.id}})))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 101, Col: 138}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var32))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 50, "\" hx-swap=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var33 string
						templ_7745c5c3_Var33, templ_7745c5c3_Err = templ.ResolveAttributeValue(keepPlace)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 101, Col: 160}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var33)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 51, "\" title=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var34 string
						templ_7745c5c3_Var34, templ_7745c5c3_Err = templ.ResolveAttributeValue(r.s.Span.Name)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 101, Col: 184}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var34)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 52, "\">")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var35 string
						templ_7745c5c3_Var35, templ_7745c5c3_Err = templ.JoinStringErrs(r.s.Span.Name)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 101, Col: 202}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var35))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 53, "</a> ")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						if r.toggle != "" && !r.open {
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 54, "<span class=\"shrink-0 text-[11px] text-muted\">+")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var36 string
							templ_7745c5c3_Var36, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(r.below))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 103, Col: 73}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var36))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 55, "</span>")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 56, "</span> <svg class=\"span-track\" viewBox=\"0 0 100 6\" preserveAspectRatio=\"none\" aria-hidden=\"true\">")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var37 = []any{"span-bar", "tone-" + string(spanTone(r.s.Span.Status)), templ.KV("span-marker", r.marker)}
						templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var37...)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 57, "<rect x=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var38 string
						templ_7745c5c3_Var38, templ_7745c5c3_Err = templ.ResolveAttributeValue(pct(r.offset))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 107, Col: 33}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var38)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 58, "\" y=\"0\" width=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var39 string
						templ_7745c5c3_Var39, templ_7745c5c3_Err = templ.ResolveAttributeValue(pct(r.width))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 107, Col: 62}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var39)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 59, "\" height=\"6\" class=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var40 string
						templ_7745c5c3_Var40, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var37).String())
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var40)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 60, "\"></rect></svg> <span class=\"text-xs whitespace-nowrap text-muted\">")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var41 string
						templ_7745c5c3_Var41, templ_7745c5c3_Err = templ.JoinStringErrs(durationText(r.duration))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 110, Col: 36}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var41))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 61, " · ")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var42 string
						templ_7745c5c3_Var42, templ_7745c5c3_Err = templ.JoinStringErrs(string(r.s.Span.Status))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 110, Col: 67}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var42))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 62, " ")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						if r.sourceMark != "" {
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 63, "· time ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var43 string
							templ_7745c5c3_Var43, templ_7745c5c3_Err = templ.JoinStringErrs(r.sourceMark)
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 112, Col: 33}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var43))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 64, " ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						}
						if p := placement(r.s.Seq, snap); p != "" {
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 65, "· ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var44 string
							templ_7745c5c3_Var44, templ_7745c5c3_Err = templ.JoinStringErrs(p)
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 115, Col: 17}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var44))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 66, "</span></li>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 67, "</ol>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					if r := pickSpan(rows, st.query.Get("span")); r != nil {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 68, "<aside class=\"card split-detail space-y-3 text-sm\" aria-label=\"Span detail\"><div class=\"flex items-start gap-2\"><h3 class=\"min-w-0 font-semibold break-words\">")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var45 string
						templ_7745c5c3_Var45, templ_7745c5c3_Err = templ.JoinStringErrs(r.s.Span.Name)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 124, Col: 70}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var45))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 69, "</h3><a class=\"close-x\" href=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var46 templ.SafeURL
						templ_7745c5c3_Var46, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.href(map[string][]string{"span": nil})))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 125, Col: 91}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var46))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 70, "\" hx-swap=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var47 string
						templ_7745c5c3_Var47, templ_7745c5c3_Err = templ.ResolveAttributeValue(keepPlace)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 125, Col: 113}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var47)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 71, "\" aria-label=\"Close the span detail\">×</a></div><dl class=\"kv kv-tight\"><dt>span</dt><dd class=\"break-all\">")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var48 string
						templ_7745c5c3_Var48, templ_7745c5c3_Err = templ.JoinStringErrs(r.id)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 129, Col: 37}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var48))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 72, "</dd><dt>start</dt><dd>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var49 string
						templ_7745c5c3_Var49, templ_7745c5c3_Err = templ.JoinStringErrs(r.s.Span.Start)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 131, Col: 29}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var49))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 73, "</dd><dt>end</dt><dd>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var50 string
						templ_7745c5c3_Var50, templ_7745c5c3_Err = templ.JoinStringErrs(orElse(r.s.Span.End, "not ended"))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 133, Col: 48}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var50))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 74, "</dd><dt>status</dt><dd>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var51 string
						templ_7745c5c3_Var51, templ_7745c5c3_Err = templ.JoinStringErrs(string(r.s.Span.Status))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 135, Col: 38}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var51))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 75, "</dd><dt>time source</dt><dd>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var52 string
						templ_7745c5c3_Var52, templ_7745c5c3_Err = templ.JoinStringErrs(string(r.s.TSSource))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 137, Col: 35}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var52))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 76, "</dd>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						for _, k := range sortedKeys(r.s.Attrs) {
							if !isRef(k) {
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 77, "<dt class=\"break-all\">")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
								var templ_7745c5c3_Var53 string
								templ_7745c5c3_Var53, templ_7745c5c3_Err = templ.JoinStringErrs(k)
								if templ_7745c5c3_Err != nil {
									return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 140, Col: 36}
								}
								_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var53))
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 78, "</dt><dd class=\"break-all\">")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
								var templ_7745c5c3_Var54 string
								templ_7745c5c3_Var54, templ_7745c5c3_Err = templ.JoinStringErrs(attrText(r.s.Attrs[k]))
								if templ_7745c5c3_Err != nil {
									return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 141, Col: 57}
								}
								_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var54))
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
								templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 79, "</dd>")
								if templ_7745c5c3_Err != nil {
									return templ_7745c5c3_Err
								}
							}
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 80, "</dl>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = signalDetail(st, r.s).Render(ctx, templ_7745c5c3_Buffer)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						for _, l := range r.s.Span.Links {
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 81, "<a class=\"link block text-xs\" href=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var55 templ.SafeURL
							templ_7745c5c3_Var55, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.href(map[string][]string{"trace": {l.TraceID}, "span": nil, "open": nil})))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 147, Col: 137}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var55))
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 82, "\">Follow the link to ")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var56 string
							templ_7745c5c3_Var56, templ_7745c5c3_Err = templ.JoinStringErrs(l.TraceID)
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 147, Col: 170}
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
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 84, "</aside>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 85, "</div></div></div>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
			}
		}
		return nil
	})
}

func foldTitle(r foldedSpan) string {
	if r.open {
		return "Fold the " + itoa(r.below) + " spans under " + r.s.Span.Name
	}
	return "Show the " + itoa(r.below) + " spans under " + r.s.Span.Name
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
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
		templ_7745c5c3_Var57 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var57 == nil {
			templ_7745c5c3_Var57 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		if list := st.markSteps(tables(all)); true {
			templ_7745c5c3_Err = emptyNotice(st, snap, len(list)).Render(ctx, templ_7745c5c3_Buffer)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			for _, t := range list {
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 86, "<section class=\"space-y-2\" aria-label=\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var58 string
				templ_7745c5c3_Var58, templ_7745c5c3_Err = templ.ResolveAttributeValue("table " + t.name)
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 186, Col: 60}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var58)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 87, "\"><h3 class=\"text-sm font-semibold\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var59 string
				templ_7745c5c3_Var59, templ_7745c5c3_Err = templ.JoinStringErrs(t.name)
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 187, Col: 46}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var59))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 88, " <span class=\"font-normal text-muted\">latest ")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var60 string
				templ_7745c5c3_Var60, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(len(t.rows)))
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 187, Col: 112}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var60))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 89, " of ")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var61 string
				templ_7745c5c3_Var61, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(t.total))
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 187, Col: 133}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var61))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 90, " rows</span></h3><div class=\"flex flex-wrap gap-1\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				for _, c := range t.columns {
					var templ_7745c5c3_Var62 = []any{"pill", templ.KV("pill-on", !slices.Contains(st.query["hide"], t.name+"."+c))}
					templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var62...)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 91, "<a href=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var63 templ.SafeURL
					templ_7745c5c3_Var63, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.toggle("hide", t.name+"."+c)))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 190, Col: 62}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var63))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 92, "\" class=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var64 string
					templ_7745c5c3_Var64, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var62).String())
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var64)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 93, "\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var65 string
					templ_7745c5c3_Var65, templ_7745c5c3_Err = templ.JoinStringErrs(c)
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 190, Col: 156}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var65))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 94, "</a>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 95, "</div><div class=\"card overflow-x-auto p-0\"><table class=\"grid-table\"><thead><tr><th>time</th>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				if t.steps {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 96, "<th>step</th>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				for _, c := range visible(st, t) {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 97, "<th>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var66 string
					templ_7745c5c3_Var66, templ_7745c5c3_Err = templ.JoinStringErrs(c)
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 202, Col: 16}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var66))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 98, "</th>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 99, "<th>bundle</th></tr></thead> <tbody>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				for i, s := range t.rows {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 100, "<tr><td class=\"font-mono text-xs\"><a class=\"link\" href=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var67 templ.SafeURL
					templ_7745c5c3_Var67, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.signalHref(s)))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 210, Col: 93}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var67))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 101, "\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var68 string
					templ_7745c5c3_Var68, templ_7745c5c3_Err = templ.JoinStringErrs(clock(s.TS))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 210, Col: 109}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var68))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 102, "</a></td>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					if t.steps {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 103, "<td>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = stepLinks(st, st.correlation().stepsOf(s)).Render(ctx, templ_7745c5c3_Buffer)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 104, "</td>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					for _, c := range visible(st, t) {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 105, "<td>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var69 string
						templ_7745c5c3_Var69, templ_7745c5c3_Err = templ.JoinStringErrs(cellOf(s, t.bodies[i], c))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 217, Col: 41}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var69))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 106, "</td>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 107, "<td class=\"text-xs text-muted\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var70 string
					templ_7745c5c3_Var70, templ_7745c5c3_Err = templ.JoinStringErrs(placement(s.Seq, snap))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 219, Col: 64}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var70))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 108, "</td></tr>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 109, "</tbody></table></div></section>")
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
		templ_7745c5c3_Var71 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var71 == nil {
			templ_7745c5c3_Var71 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		if sources := logSources(snap.signals); true {
			templ_7745c5c3_Err = emptyNotice(st, snap, len(sources)).Render(ctx, templ_7745c5c3_Buffer)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			if src := pickSource(sources, st.query.Get("source")); src != nil {
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 110, "<div class=\"flex flex-wrap gap-1\" aria-label=\"Log sources\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				for _, s := range sources {
					var templ_7745c5c3_Var72 = []any{"pill", templ.KV("pill-on", s.name == src.name)}
					templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var72...)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 111, "<a href=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var73 templ.SafeURL
					templ_7745c5c3_Var73, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.href(map[string][]string{"source": {s.name}})))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 258, Col: 78}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var73))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 112, "\" class=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var74 string
					templ_7745c5c3_Var74, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var72).String())
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 1, Col: 0}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var74)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 113, "\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var75 string
					templ_7745c5c3_Var75, templ_7745c5c3_Err = templ.JoinStringErrs(s.name)
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 258, Col: 147}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var75))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 114, " · ")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var76 string
					templ_7745c5c3_Var76, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(len(s.lines)))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 258, Col: 173}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var76))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 115, "</a>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 116, "</div><div class=\"console log-box\"><div>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				for _, l := range lastN(src.lines, logWindow) {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 117, "<div><span class=\"text-muted\">")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var77 string
					templ_7745c5c3_Var77, templ_7745c5c3_Err = templ.JoinStringErrs(clock(l.TS))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 264, Col: 49}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var77))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 118, "</span> ")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var78 string
					templ_7745c5c3_Var78, templ_7745c5c3_Err = templ.JoinStringErrs(logBody(l))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `signalviews.templ`, Line: 264, Col: 71}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var78))
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 119, "</div>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 120, "</div></div>")
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
