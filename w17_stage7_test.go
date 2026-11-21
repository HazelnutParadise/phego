package phego

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// W-17 stage 7A: REQ-001, REQ-016, REQ-025, REQ-026; T-009, T-010,
// T-029..T-032. Through the public API: the classes, objects, static
// properties, class constants and conditionally declared classes of one run
// never reach another run of the same Program, and temporaries holding
// objects are destroyed at PHP's time in functions of any size.

const w17Stage7Program = `<?php
interface Named { const PREFIX = "n:"; public function name(); }
trait Counts {
    private static $made = 0;
    public static function made() { return self::$made; }
    protected function count() { static $calls = 0; $calls++; self::$made++; return $calls; }
}
abstract class Base implements Named {
    use Counts;
    const TABLE = [self::PREFIX . "a", self::PREFIX . "b"];
    public static $log = [];
    protected $tags = [self::PREFIX];
    public function __construct(protected string $id) { static::$log[] = "new " . $id; $this->count(); }
    public function __destruct() { static::$log[] = "gone " . $this->id; echo "<", $this->id, ">"; }
    public function name() { return static::class . ":" . $this->id . ":" . self::TABLE[1]; }
}
if ($variant) {
    class Leaf extends Base { const KIND = "odd"; public function tag($t) { $this->tags[] = $t; return $this; } public function tags() { $o = ""; foreach ($this->tags as $t) { $o .= $t . ","; } return $o; } }
} else {
    class Leaf extends Base { const KIND = "even"; public static $log = ["own"]; public function tag($t) { $this->tags[] = "E" . $t; return $this; } public function tags() { $o = ""; foreach ($this->tags as $t) { $o .= $t . ";"; } return $o; } }
}
$a = new Leaf($host);
$b = new Leaf($host . "2");
echo $a->name(), " ", Leaf::KIND, " ", $a->tag("x")->tag($host)->tags(), " ", Leaf::made(), "\n";
$b = null;
$n = 0;
foreach (Leaf::$log as $l) { $n++; }
echo "log ", $n, " ", Base::$log === Leaf::$log ? "shared" : "own", "\n";
`

func TestW17Stage7StateBelongsToTheRun(t *testing.T) {
	e := mustEngine(t, Config{})
	p, err := e.Compile(context.Background(), "/w/stage7.php", w17Stage7Program)
	if err != nil {
		t.Fatal(err)
	}
	run := func(id int) error {
		host := fmt.Sprintf("h%d", id)
		odd := id%2 == 1
		r, err := e.Run(context.Background(), p, WithGlobal("host", Bytes([]byte(host))), WithGlobal("variant", Bool(odd)))
		want := fmt.Sprintf("Leaf:%s:n:b odd n:,x,%s, 2\n<%s2>log 3 shared\n<%s>", host, host, host, host)
		if !odd {
			want = fmt.Sprintf("Leaf:%s:n:b even n:;Ex;E%s; 2\n<%s2>log 4 own\n<%s>", host, host, host, host)
		}
		if err != nil || r.Status != StatusCompleted || string(r.Output) != want {
			return fmt.Errorf("run %d = %+v, %v; want output %q", id, r, err, want)
		}
		return nil
	}
	for id := 1; id <= 4; id++ {
		if err := run(id); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if err := run(g*1000 + i); err != nil {
					errs <- err
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// w17LargePadding is PHP code for the first line of a file that makes
// {main} too large for the VM's bit-set liveness analysis, as a huge PHP
// function is: one call with very many arguments (many registers) and tens of
// thousands of statements. It adds no line, output or object.
func w17LargePadding() string {
	var b strings.Builder
	// A call evaluates each argument into a register of its own before it
	// sends any: 1,100 arguments hold 1,100 registers.
	b.WriteString(" function w17pad(...$a) {} $w17pad = 0; w17pad(")
	const args = 1100
	for i := 0; i < args; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("$w17pad+1")
	}
	b.WriteString(");")
	for i := 0; i < 40000; i++ {
		b.WriteString(" $w17pad = 1;")
	}
	return b.String()
}

// The destructor fixtures give the same output when {main} is made too
// large for the ordinary liveness analysis: a temporary object is released
// where PHP releases it whatever the size of the function.
func TestW17Stage7LargeFunctionDestructorTiming(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("tests", "vm", "w17", "reference-64.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []struct {
			Case    string `json:"case"`
			Display struct {
				Stdout string `json:"stdout_b64"`
				Exit   int    `json:"exit_code"`
			} `json:"display"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for _, c := range ref.Cases {
		out, err := base64.StdEncoding.DecodeString(c.Display.Stdout)
		if err != nil {
			t.Fatal(err)
		}
		want[c.Case] = string(out)
	}
	e := mustEngine(t, Config{IntSize: 64})
	pad := w17LargePadding()
	for _, name := range []string{"magic_method_destruct_scope", "magic_method_destruct_unset", "magic_method_destruct_output_order", "magic_method_destruct_end_order", "object_in_arrays", "method_chaining_fluent"} {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("tests", "vm", "w17", "cases", name+".php"))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := want[name]; !ok || !strings.HasPrefix(string(src), "<?php\n") {
				t.Fatalf("fixture %s is missing from the reference or does not start with a bare opening tag", name)
			}
			big := "<?php" + pad + string(src[len("<?php"):])
			o := w17Execute(t, e, name, big, WithINI("display_errors", "1"), WithINI("log_errors", "0"), WithINI("html_errors", "0"), WithINI("error_reporting", "-1"))
			if o.err != nil && string(o.stdout) == "" {
				t.Fatalf("run failed: %v", o.err)
			}
			if string(o.stdout) != want[name] {
				t.Errorf("output differs from PHP 8.5.12 in a large function:\n got %q\nwant %q", o.stdout, want[name])
			}
		})
	}
}

// Code the runtime calls back nests on the Go stack, so its depth is
// bounded: a destructor that creates and drops an object of its own class,
// and a __get that recurses, end with PHP's stack-limit Error instead of
// exhausting the host's stack. (PHP's own bound depends on its C stack, so
// the depth differs and these are not differential fixtures.)
func TestW17Stage7NestedCallbacksAreBounded(t *testing.T) {
	e := mustEngine(t, Config{})
	for name, src := range map[string]string{
		"destructor":  "<?php\nclass D { function __destruct() { $x = new D; } }\nnew D;\n",
		"magic get":   "<?php\nclass G { function __get($n) { return $this->$n; } }\n$g = new G;\necho $g->a;\n",
		"to string":   "<?php\nclass S { public $n; function __toString() { $s = new S; $s->n = $this->n + 1; return \"x\" . $s; } }\n$s = new S;\n$s->n = 0;\necho $s;\n",
		"constructor": "<?php\nclass C { function __construct() { new C; } }\nnew C;\n",
	} {
		t.Run(name, func(t *testing.T) {
			if name == "constructor" || name == "magic get" {
				// A constructor is an ordinary call, and __get guards its own
				// recursion: neither nests without bound. They must still end.
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, err := e.Run(ctx, w17Nested(t, e, src))
				if err == nil && name == "constructor" {
					t.Fatal("unbounded constructor recursion completed")
				}
				return
			}
			_, err := e.Run(context.Background(), w17Nested(t, e, src))
			var pe *PHPError
			if !errors.As(err, &pe) || pe.Kind != UncaughtThrowable || !strings.Contains(pe.Message, "Maximum call stack size") {
				t.Fatalf("err = %v, want PHP's stack-limit Error", err)
			}
		})
	}
}

func w17Nested(t *testing.T, e *Engine, src string) *Program {
	t.Helper()
	p, err := e.Compile(context.Background(), "/w/nested.php", src)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestW17Stage7LongObjectChainIsFreed frees a chain of objects far longer
// than any nesting the Go stack could carry: the frees past a fixed depth
// are queued. (PHP 8.5.12 itself crashes on a chain this long, so this is
// not a differential fixture.)
func TestW17Stage7LongObjectChainIsFreed(t *testing.T) {
	e := mustEngine(t, Config{})
	for name, src := range map[string]string{
		"properties": "<?php\nclass N { public $next; }\n$h = null;\nfor ($i = 0; $i < 400000; $i++) { $x = new N; $x->next = $h; $h = $x; }\n$x = null;\n$h = null;\necho \"done\\n\";\n",
		"arrays":     "<?php\nclass N { public $arr; }\n$h = null;\nfor ($i = 0; $i < 200000; $i++) { $x = new N; $x->arr = [$h]; $h = $x; }\n$x = null;\n$h = null;\necho \"done\\n\";\n",
		"at the end": "<?php\nclass N { public $next; }\n$h = null;\nfor ($i = 0; $i < 400000; $i++) { $x = new N; $x->next = $h; $h = $x; }\n$x = null;\necho \"done\\n\";\n",
	} {
		t.Run(name, func(t *testing.T) {
			r, err := e.Run(context.Background(), w17Nested(t, e, src))
			if err != nil || r.Status != StatusCompleted || string(r.Output) != "done\n" {
				t.Fatalf("status %v, output %q, err %v", r.Status, r.Output, err)
			}
		})
	}
}

// TestW17Stage7ShutdownGlobalSeesHostValue: a function a shutdown destructor
// calls binds a global {main} never names to the host's value, as it does
// while {main} runs.
func TestW17Stage7ShutdownGlobalSeesHostValue(t *testing.T) {
	e := mustEngine(t, Config{})
	src := "<?php\nfunction f() { global $g; echo \"[\", $g, \"]\\n\"; }\nclass D { function __destruct() { f(); } }\n$d = new D;\n"
	r, err := e.Run(context.Background(), w17Nested(t, e, src), WithGlobal("g", Bytes([]byte("host"))))
	if err != nil || string(r.Output) != "[host]\n" {
		t.Fatalf("output %q, err %v", r.Output, err)
	}
}
