package screen

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

import (
	"net/url"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	paneStep  = "step"
	paneCalls = "calls"
)

func flowTreeNav(t flowTree, pick treePick) templ.Component {
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
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 1, "<nav class=\"tree\" aria-label=\"Flows\" data-scroll-box=\"tree\"><div class=\"tree-filter\" aria-label=\"Steps to list\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		for _, f := range t.filters {
			var templ_7745c5c3_Var2 = []any{"pill", templ.KV("pill-on", f.on)}
			templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var2...)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 2, "<a href=\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var3 templ.SafeURL
			templ_7745c5c3_Var3, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(f.href))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 19, Col: 35}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var3))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 3, "\" class=\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var4 string
			templ_7745c5c3_Var4, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var2).String())
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 1, Col: 0}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var4)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 4, "\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var5 string
			templ_7745c5c3_Var5, templ_7745c5c3_Err = templ.JoinStringErrs(f.label)
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 19, Col: 91}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var5))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 5, " <span class=\"tabular-nums\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var6 string
			templ_7745c5c3_Var6, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(f.count))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 19, Col: 136}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var6))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 6, "</span></a>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 7, "</div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if t.rows == 0 {
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 8, "<p class=\"px-3 py-3 text-xs text-muted\">No flow to list here.</p>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		for _, stage := range t.stages {
			if stage.name != "" {
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 9, "<h3 class=\"tree-stage\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var7 string
				templ_7745c5c3_Var7, templ_7745c5c3_Err = templ.JoinStringErrs(stage.name)
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 27, Col: 39}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var7))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 10, "</h3>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 11, " <ol>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			for _, f := range stage.flows {
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 12, "<li>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var8 = []any{"tree-flow", templ.KV("tree-on", f.on)}
				templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var8...)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 13, "<a href=\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var9 templ.SafeURL
				templ_7745c5c3_Var9, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(f.href))
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 32, Col: 37}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var9))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 14, "\" class=\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var10 string
				templ_7745c5c3_Var10, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var8).String())
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 1, Col: 0}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var10)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 15, "\" title=\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var11 string
				templ_7745c5c3_Var11, templ_7745c5c3_Err = templ.ResolveAttributeValue(f.id + " · " + statusLabel(f.status))
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 32, Col: 134}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var11)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 16, "\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				if f.on {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 17, " aria-current=\"true\" data-scroll-here=\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					var templ_7745c5c3_Var12 string
					templ_7745c5c3_Var12, templ_7745c5c3_Err = templ.ResolveAttributeValue(treeKey(pick))
					if templ_7745c5c3_Err != nil {
						return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 32, Col: 199}
					}
					_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var12)
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 18, "\"")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 19, ">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = statusMark(f.status).Render(ctx, templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 20, "<span class=\"min-w-0 truncate font-mono text-xs font-medium\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var13 string
				templ_7745c5c3_Var13, templ_7745c5c3_Err = templ.JoinStringErrs(f.id)
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 34, Col: 74}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var13))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 21, "</span> <span class=\"tree-meta\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var14 string
				templ_7745c5c3_Var14, templ_7745c5c3_Err = templ.JoinStringErrs(f.count)
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 35, Col: 40}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var14))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 22, "</span></a> ")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				if len(f.steps) > 0 {
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 23, "<ol>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
					for _, s := range f.steps {
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 24, "<li>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var15 = []any{"tree-step", templ.KV("tree-on", s.on)}
						templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var15...)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 25, "<a href=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var16 templ.SafeURL
						templ_7745c5c3_Var16, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(s.href))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 41, Col: 41}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var16))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 26, "\" class=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var17 string
						templ_7745c5c3_Var17, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var15).String())
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 1, Col: 0}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var17)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 27, "\" title=\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var18 string
						templ_7745c5c3_Var18, templ_7745c5c3_Err = templ.ResolveAttributeValue(orElse(s.title, s.id))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 41, Col: 122}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var18)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 28, "\"")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						if s.on {
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 29, " aria-current=\"true\" data-scroll-here=\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							var templ_7745c5c3_Var19 string
							templ_7745c5c3_Var19, templ_7745c5c3_Err = templ.ResolveAttributeValue(treeKey(pick))
							if templ_7745c5c3_Err != nil {
								return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 41, Col: 187}
							}
							_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var19)
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
							templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 30, "\"")
							if templ_7745c5c3_Err != nil {
								return templ_7745c5c3_Err
							}
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 31, ">")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = statusMark(s.status).Render(ctx, templ_7745c5c3_Buffer)
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 32, "<span class=\"min-w-0 truncate\">")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var20 string
						templ_7745c5c3_Var20, templ_7745c5c3_Err = templ.JoinStringErrs(orElse(s.title, s.id))
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 43, Col: 65}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var20))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 33, "</span> <span class=\"tree-meta\">")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						var templ_7745c5c3_Var21 string
						templ_7745c5c3_Var21, templ_7745c5c3_Err = templ.JoinStringErrs(s.meta)
						if templ_7745c5c3_Err != nil {
							return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 44, Col: 43}
						}
						_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var21))
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
						templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 34, "</span></a></li>")
						if templ_7745c5c3_Err != nil {
							return templ_7745c5c3_Err
						}
					}
					templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 35, "</ol>")
					if templ_7745c5c3_Err != nil {
						return templ_7745c5c3_Err
					}
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 36, "</li>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 37, "</ol>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		if t.outside != nil {
			var templ_7745c5c3_Var22 = []any{"tree-flow", "mt-2", "border-t", "border-line", templ.KV("tree-on", t.outside.on)}
			templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var22...)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 38, "<a href=\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var23 templ.SafeURL
			templ_7745c5c3_Var23, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(t.outside.href))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 55, Col: 42}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var23))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 39, "\" class=\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var24 string
			templ_7745c5c3_Var24, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var22).String())
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 1, Col: 0}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var24)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 40, "\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			if t.outside.on {
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 41, " aria-current=\"true\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 42, "><span class=\"min-w-0 truncate\">Outside any step</span> <span class=\"tree-meta\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var25 string
			templ_7745c5c3_Var25, templ_7745c5c3_Err = templ.JoinStringErrs(itoa(t.outside.calls))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 57, Col: 51}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var25))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 43, "</span></a>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 44, "</nav>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return nil
	})
}

func flowFacts(f contract.FlowSummary) string {
	facts := []string{"stage " + f.Stage, "requires " + requiresText(f), onceText(f.Once), itoa(f.Passed) + " of " + itoa(f.Steps) + " steps passed"}
	if f.Stage == "" {
		facts = facts[1:]
	}
	return strings.Join(facts, " · ")
}

func onceText(once bool) string {
	if once {
		return "runs once per environment"
	}
	return "can run again"
}

func findFlow(flows []contract.FlowSummary, id string) *contract.FlowSummary {
	for i := range flows {
		if flows[i].ID == id {
			return &flows[i]
		}
	}
	return nil
}

func (st *state) callsHref(flow, step string) string {
	q := url.Values{"flow": {flow}}
	if step == "" {
		q.Set("scope", "flow")
	} else {
		q.Set("step", step)
	}
	if only := st.only(); only != "" {
		q.Set("only", only)
	}
	return "/runs/" + st.run.ID + "/wire?" + q.Encode()
}

func pickHead(st *state, f contract.FlowSummary, s *contract.FlowStep, source string) templ.Component {
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
		templ_7745c5c3_Var26 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var26 == nil {
			templ_7745c5c3_Var26 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 45, "<div class=\"space-y-2\"><p class=\"flex flex-wrap items-center gap-1.5 font-mono text-xs text-muted\"><a class=\"hover:text-fg\" href=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var27 templ.SafeURL
		templ_7745c5c3_Var27, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.flowHref(f.ID, "", "")))
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 103, Col: 75}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var27))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 46, "\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var28 string
		templ_7745c5c3_Var28, templ_7745c5c3_Err = templ.JoinStringErrs(f.ID)
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 103, Col: 84}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var28))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 47, "</a> ")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if s != nil {
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 48, "<span>›</span> <span class=\"text-fg\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var29 string
			templ_7745c5c3_Var29, templ_7745c5c3_Err = templ.JoinStringErrs(s.ID)
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 106, Col: 32}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var29))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 49, "</span> ")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		if source != "" {
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 50, "<span class=\"ml-auto font-sans\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = sourceTag(source).Render(ctx, templ_7745c5c3_Buffer)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 51, "</span>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 52, "</p>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if s != nil {
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 53, "<div class=\"flex flex-wrap items-start gap-2\"><h2 class=\"detail-title\" title=\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var30 string
			templ_7745c5c3_Var30, templ_7745c5c3_Err = templ.ResolveAttributeValue(orElse(s.Title, s.ID))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 116, Col: 58}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var30)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 54, "\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var31 string
			templ_7745c5c3_Var31, templ_7745c5c3_Err = templ.JoinStringErrs(orElse(s.Title, s.ID))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 116, Col: 84}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var31))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 55, "</h2>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = badge(flowTone(string(s.Status)), statusLabel(string(s.Status))).Render(ctx, templ_7745c5c3_Buffer)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 56, "</div><p class=\"text-xs text-muted\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var32 string
			templ_7745c5c3_Var32, templ_7745c5c3_Err = templ.JoinStringErrs(string(s.Kind))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 119, Col: 49}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var32))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 57, " step · ")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var33 string
			templ_7745c5c3_Var33, templ_7745c5c3_Err = templ.JoinStringErrs(stepFacts(*s))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 119, Col: 75}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var33))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 58, "</p>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			if s.Message != "" {
				var templ_7745c5c3_Var34 = []any{templ.KV("failure", s.Status == contract.StepStatusFailed), templ.KV("console", s.Status != contract.StepStatusFailed)}
				templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var34...)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 59, "<pre class=\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var35 string
				templ_7745c5c3_Var35, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var34).String())
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 1, Col: 0}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var35)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 60, "\">")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				var templ_7745c5c3_Var36 string
				templ_7745c5c3_Var36, templ_7745c5c3_Err = templ.JoinStringErrs(s.Message)
				if templ_7745c5c3_Err != nil {
					return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 121, Col: 149}
				}
				_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var36))
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 61, "</pre>")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
			}
		} else {
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 62, "<div class=\"flex flex-wrap items-start gap-2\"><h2 class=\"detail-title\" title=\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var37 string
			templ_7745c5c3_Var37, templ_7745c5c3_Err = templ.ResolveAttributeValue(orElse(f.Title, f.ID))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 125, Col: 58}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var37)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 63, "\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var38 string
			templ_7745c5c3_Var38, templ_7745c5c3_Err = templ.JoinStringErrs(orElse(f.Title, f.ID))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 125, Col: 84}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var38))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 64, "</h2>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = badge(flowTone(string(f.Status)), statusLabel(string(f.Status))).Render(ctx, templ_7745c5c3_Buffer)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 65, "</div><p class=\"text-xs text-muted\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var39 string
			templ_7745c5c3_Var39, templ_7745c5c3_Err = templ.JoinStringErrs(flowFacts(f))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 128, Col: 47}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var39))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 66, "</p>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 67, "</div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return nil
	})
}

func ranIn(f contract.FlowSummary, s *contract.FlowStep) bool {
	if s != nil {
		return s.Status != contract.StepStatusSkipped
	}
	return f.Steps == 0 || f.Skipped < f.Steps
}

func paneTabs(st *state, flow, step, on string, calls bool) templ.Component {
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
		templ_7745c5c3_Var40 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var40 == nil {
			templ_7745c5c3_Var40 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 68, "<nav class=\"subtabs\" aria-label=\"Panes\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var41 = []any{"subtab", templ.KV("subtab-on", on == paneStep)}
		templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var41...)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 69, "<a href=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var42 templ.SafeURL
		templ_7745c5c3_Var42, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.flowHref(flow, step, "")))
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 142, Col: 54}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var42))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 70, "\" class=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var43 string
		templ_7745c5c3_Var43, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var41).String())
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 1, Col: 0}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var43)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 71, "\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if on == paneStep {
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 72, " aria-current=\"page\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 73, ">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if step == "" {
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 74, "Steps")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		} else {
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 75, "Step")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 76, "</a> ")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if st.run.Wire && calls {
			var templ_7745c5c3_Var44 = []any{"subtab", templ.KV("subtab-on", on == paneCalls)}
			templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var44...)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 77, "<a href=\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var45 templ.SafeURL
			templ_7745c5c3_Var45, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.callsHref(flow, step)))
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 150, Col: 52}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var45))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 78, "\" class=\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var46 string
			templ_7745c5c3_Var46, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var44).String())
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 1, Col: 0}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var46)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 79, "\" title=\"What each server was asked, and what it answered\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			if on == paneCalls {
				templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 80, " aria-current=\"page\"")
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
			}
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 81, ">Calls</a> ")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		var templ_7745c5c3_Var47 = []any{"subtab", templ.KV("subtab-on", on == paneSource)}
		templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var47...)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 82, "<a href=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var48 templ.SafeURL
		templ_7745c5c3_Var48, templ_7745c5c3_Err = templ.JoinURLErrs(templ.SafeURL(st.flowHref(flow, step, paneSource)))
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 152, Col: 62}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var48))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 83, "\" class=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var49 string
		templ_7745c5c3_Var49, templ_7745c5c3_Err = templ.ResolveAttributeValue(templ.CSSClasses(templ_7745c5c3_Var47).String())
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `flows.templ`, Line: 1, Col: 0}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ_7745c5c3_Var49)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 84, "\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if on == paneSource {
			templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 85, " aria-current=\"page\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 86, ">Source</a></nav>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return nil
	})
}

func sourceTag(source string) templ.Component {
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
		templ_7745c5c3_Var50 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var50 == nil {
			templ_7745c5c3_Var50 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		if tone, text := sourceBadge(source); true {
			templ_7745c5c3_Err = badge(tone, text).Render(ctx, templ_7745c5c3_Buffer)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		return nil
	})
}

var _ = templruntime.GeneratedTemplate
