<?php
// REQ-018/020/021; T-035/036/039-042. Dynamic property churn preserves visible names and references.
$tests ??= [];

#[AllowDynamicProperties]
class W13R9Dynamic {}

function w13R9Probe(bool $detached): array {
    $object = new W13R9Dynamic();
    $object->anchor = 1;
    $alias =& $object->anchor;
    for ($i = 0; $i < 128; ++$i) {
        $object->x = $i;
        unset($object->x);
        $tmp = 'tmp' . $i;
        $object->$tmp = $i;
        unset($object->$tmp);
    }
    if ($detached) {
        unset($object->anchor);
        $object->anchor = 3;
    }
    $alias = 7;
    $object->tail = 2;
    $clone = clone $object;
    $clone->tail = 9;
    $result = [array_keys((array)$object), $object->anchor, $object->tail,
        array_keys((array)$clone), $clone->anchor, $clone->tail, $alias];
    unset($alias, $clone, $object);
    return $result;
}

$w13R9Names = ['r9-dynamic-churn-detached', 'r9-dynamic-churn-shared'];
$tests['r9-dynamic-churn-detached'] = fn() => w13R9Probe(true);
$tests['r9-dynamic-churn-shared'] = fn() => w13R9Probe(false);

if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $cases = [];
    foreach ($w13R9Names as $name) {
        $diagnostics = [];
        set_error_handler(function ($level, $message) use (&$diagnostics) { $diagnostics[] = [$level, $message]; return true; });
        try {
            $result = $tests[$name]();
        } catch (Throwable $e) {
            $result = [$e::class, $e->getMessage()];
        }
        restore_error_handler();
        $cases[$name] = ['result' => $result, 'diagnostics' => $diagnostics];
    }
    echo json_encode(['php' => PHP_VERSION, 'int_size' => PHP_INT_SIZE, 'cases' => $cases], JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT), "\n";
}
