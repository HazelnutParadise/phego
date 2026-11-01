<?php
// Includes a fixture and prints what PHP reports about it through
// Reflection, as JSON. Run in the pinned PHP 8.5.12 image:
//   php -n reflect.php /w/reflect/names.php
// TestLowerMatchesReflection in internal/ir builds the same JSON from HIR.
$fixture = $argv[1];
$values = [];
$closures = [];
ob_start();
include $fixture;
ob_end_clean();

function type_of(?ReflectionType $t): ?string { return $t === null ? null : (string)$t; }

function params(ReflectionFunctionAbstract $f): array {
    $out = [];
    foreach ($f->getParameters() as $p) {
        $out[] = [
            'name' => $p->getName(),
            'type' => type_of($p->getType()),
            'byRef' => $p->isPassedByReference(),
            'variadic' => $p->isVariadic(),
            'optional' => $p->isOptional(),
            'default' => $p->isDefaultValueAvailable(),
            'promoted' => $p->isPromoted(),
        ];
    }
    return $out;
}

function fn_info(ReflectionFunctionAbstract $f): array {
    return [
        'name' => $f->getName(),
        'returnType' => type_of($f->getReturnType()),
        'byRef' => $f->returnsReference(),
        'generator' => $f->isGenerator(),
        'variadic' => $f->isVariadic(),
        'statics' => array_keys($f->getStaticVariables()),
        'params' => params($f),
    ];
}

function value(mixed $v): array {
    return match (true) {
        is_string($v) => ['string', bin2hex($v)],
        is_int($v) => ['int', (string)$v],
        is_float($v) => ['float', bin2hex(pack('E', $v))],
        is_bool($v) => ['bool', $v ? 'true' : 'false'],
        $v === null => ['null', ''],
        default => ['other', get_debug_type($v)],
    };
}

$out = ['functions' => [], 'classes' => [], 'closures' => [], 'values' => array_map('value', $values)];
foreach (get_defined_functions()['user'] as $name) {
    $f = new ReflectionFunction($name);
    if ($f->getFileName() === $fixture) {
        $out['functions'][] = fn_info($f);
    }
}
$declared = [];
foreach (array_merge(get_declared_classes(), get_declared_interfaces(), get_declared_traits()) as $name) {
    $c = new ReflectionClass($name);
    if ($c->getFileName() === $fixture) {
        $declared[] = $c;
    }
}
usort($declared, fn($a, $b) => $a->getStartLine() <=> $b->getStartLine());
foreach ($declared as $c) {
    $methods = [];
    foreach ($c->getMethods() as $m) {
        if ($m->getDeclaringClass()->getName() === $c->getName()) {
            $methods[] = fn_info($m) + ['static' => $m->isStatic()];
        }
    }
    $out['classes'][] = ['name' => $c->getName(), 'methods' => $methods];
}
foreach ($closures as $c) {
    $f = new ReflectionFunction($c);
    $out['closures'][] = fn_info($f) + ['static' => $f->isStatic()];
}
echo json_encode($out, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES), "\n";
