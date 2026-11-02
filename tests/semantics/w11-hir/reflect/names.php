<?php
namespace Lib {
    class Base {}
}

namespace Lib\Contracts {
    interface Shape {}
    interface Named {}
}

namespace App\Models {
    use Lib\Base as B;
    use Lib\Contracts\{Shape, Named as N};
    use function Lib\helper as h, strlen;
    use const Lib\LIMIT, Lib\OTHER as O;

    interface Local {}

    function typed(int|string|null $a, ?B $b, Shape&N $c, iterable $d = [], float $e = 1, mixed $f = null): ?int {}
    function nullable(\Foo $a = null, int ...$rest): ?Local {}
    function &ref(array &$a, $b = PHP_INT_MAX, callable $c = null) { static $s1, $s2 = 2; return $a; }
    function gen(): \Generator { yield 1; }
    function optional($a = 1, $b, $c = null) {}
    function falsy(false|int $a, true $b, null|\stdClass $c, int|bool|B|null $d = 1, (Shape&N)|string $e = 'x'): void {}

    class Item extends B implements Local {
        public function __construct(public readonly int $id, protected ?string $name = null, $plain = []) {}
        public static function make(self $other, parent $p): static { return new static(1); }
        private function &pick(Item|Shape $x, bool $flag = false): ?self { static $memo; return $x; }
    }

    $values[] = Item::class;
    $values[] = B::class;
    $values[] = N::class;
    $values[] = \Other\Thing::class;
    $values[] = namespace\Sub\X::class;
    $values[] = B\Deep::class;
    $values[] = __NAMESPACE__;
    $values[] = __LINE__;
    $values[] = __FILE__;
    $values[] = __DIR__;
    $values[] = __FUNCTION__;
    $values[] = __CLASS__;
}
