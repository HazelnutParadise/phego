<?php
// Systematic grammar grid: expression forms x postfix suffixes, and name
// positions x keywords. Writes one file per case into /out.
$forms = ['$a', '$$a', '${"a"}', 'A', '\A', 'A\B', 'namespace\A', 'a()', '$a()', '$a->b', '$a?->b', '$a->b()', 'A::b', 'A::$b', 'A::b()', 'A::class', '$a[0]', '"s"', "'s'", 'b"s"', '"s$a"', '`echo x`', '<<<E' . "\nx\nE", '1', '1.5', '[1]', 'array(1)', '(1)', 'new A', 'new A()', '(new A)', 'clone $a', 'clone($a)', 'isset($a)', 'empty($a)', 'eval("1;")', 'exit()', 'exit', 'print 1', 'include "x"', 'function(){}', 'fn() => 1', 'static fn() => 1', 'match(1){default=>1}', '__LINE__', '__CLASS__', 'static::A', 'self::A', 'parent::A', 'list($a)', '$a++', '++$a', '-1', '!$a', '(int)$a', '@$a', '$a = 1', 'yield', 'throw $e', 'A::{"b"}', '$a::b', '$a::$b', "new class {}"];
$suffixes = ['[0]', '{0}', '->x', '?->x', '::X', '::$x', '()', '::class', '::x()', '++'];
$keywords = ['abstract','and','array','as','break','callable','case','catch','class','clone','const','continue','declare','default','do','echo','else','elseif','empty','enddeclare','endfor','endforeach','endif','endswitch','endwhile','enum','eval','exit','die','extends','final','finally','fn','for','foreach','function','global','goto','if','implements','include','include_once','instanceof','insteadof','interface','isset','list','match','namespace','new','or','print','private','protected','public','readonly','require','require_once','return','static','switch','throw','trait','try','unset','use','var','while','xor','yield','__CLASS__','__DIR__','__FILE__','__FUNCTION__','__LINE__','__METHOD__','__NAMESPACE__','__TRAIT__','__PROPERTY__','__halt_compiler','self','parent','null','true','false','int','string','mixed','void','never','object','iterable','bool','float','get','set','from'];
$positions = [
  'function K() {}', 'class K {}', 'interface K {}', 'trait K {}', 'enum K {}', 'const K = 1;',
  'class C { function K() {} }', 'class C { const K = 1; }', 'class C { public $x { K; } }',
  'enum E { case K; }', 'f(K: 1);', '$a->K;', '$a->K();', 'A::K;', 'A::K();', 'goto K;', 'K: ;',
  'namespace K;', 'use A as K;', 'use A\K;', 'class C { use T { a as K; } }', 'class C { use T { K as b; } }',
  'function f(K $x) {}', 'function f(): K {}', 'new K;', '$x instanceof K;', 'K::class;', 'K();', 'class C extends K {}',
];
$n = 0;
$put = function ($src) use (&$n) { file_put_contents(sprintf('/out/%016x.php', $n++), "<?php\n" . $src); };
foreach ($forms as $f) foreach ($suffixes as $s) $put($f . $s . ";\n");
foreach ($forms as $f) $put($f . " = 1;\n");
foreach ($positions as $p) foreach ($keywords as $k) $put(str_replace('K', $k, $p) . "\n");
echo $n, "\n";
