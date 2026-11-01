<?php
$types = ['int','?int','int|string','A&B','(A&B)|C','?A|B','static','?static','null','false','true','never','void','mixed','array','callable','self','parent','iterable','A|(B&C)','(A|B)&C','A&(B|C)','?(A&B)','A|null|false','\A','A\B','namespace\A','?array','int|','|int','A&','(A&B)','(A)|B','static|int','array|callable'];
$typePos = ['function f(T $x) {}', 'function f(): T {}', 'class C { public T $x; }', 'class C { const T X = 1; }', 'fn(T $x) => 1;', 'function f(T ...$x) {}', 'function f(T &$x) {}', 'class C { function __construct(public T $x) {} }', 'class C { public T $x { get; } }', 'try {} catch (T $e) {}', '$x instanceof T;'];
$mods = ['public','protected','private','static','abstract','final','readonly','var','public(set)','private(set)','protected(set)'];
$members = ['function f() {}', '$x;', 'const X = 1;', 'int $x { get; }', 'function __construct(M $x) {}'];
$prefix = ['new','clone','print','yield','yield from','throw','include','!','-','~','@','(int)','++','--','&','(void)','static','function','fn','match'];
$operands = ['$a','1','A','A()','$a[0]','$a->b','(1)','[1]','new A','"s"','$a = 1','fn() => 1'];
$n = 0;
$put = function ($src) use (&$n) { file_put_contents(sprintf('/out/%016x.php', $n++), "<?php\n" . $src . "\n"); };
foreach ($typePos as $p) foreach ($types as $t) $put(str_replace('T', $t, $p));
foreach ($members as $m) foreach ($mods as $a) { $put("class C { $a " . str_replace('M', '', $m) . " }"); foreach ($mods as $b) $put("class C { $a $b " . str_replace('M', '', $m) . " }"); }
foreach ($mods as $a) $put("class C { function __construct($a \$x) {} }");
foreach ($prefix as $p) foreach ($operands as $o) $put("$p $o;");
foreach (['break','continue'] as $k) foreach (['','1','$a','0','-1','(1)','2*3','1.5','"1"'] as $v) $put("while (1) { $k $v; }");
foreach (['','1','$a','0','-1'] as $v) $put("declare(ticks=$v);");
echo $n, "\n";
