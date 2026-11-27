<?php
// exception_previous_chain: three exceptions linked through the previous argument, walked with getPrevious
class LevelOne extends Exception {}
class LevelTwo extends RuntimeException {}

function make_chain() {
    $a = new InvalidArgumentException("root cause", 1);
    $b = new LevelOne("middle", 2, $a);
    $c = new LevelTwo("top", 3, $b);
    return $c;
}

$top = make_chain();
$cur = $top;
$depth = 0;
while ($cur !== null) {
    echo "depth_", $depth, "_class: ", $cur::class, "\n";
    echo "depth_", $depth, "_message: ", $cur->getMessage(), "\n";
    echo "depth_", $depth, "_code: ", $cur->getCode(), "\n";
    echo "depth_", $depth, "_line: ", $cur->getLine(), "\n";
    $cur = $cur->getPrevious();
    $depth = $depth + 1;
}
echo "total_depth: ", $depth, "\n";
echo "top_is_runtime: ", ($top instanceof RuntimeException ? 1 : 0), "\n";
echo "top_is_logic: ", ($top instanceof LogicException ? 1 : 0), "\n";

$mid = $top->getPrevious();
$inner = $mid->getPrevious();
echo "inner_is_logic: ", ($inner instanceof LogicException ? 1 : 0), "\n";
echo "inner_previous_null: ", ($inner->getPrevious() === null ? 1 : 0), "\n";
echo "mid_is_levelone: ", ($mid instanceof LevelOne ? 1 : 0), "\n";
echo "same_chain_via_two_calls: ", ($top->getPrevious()->getPrevious() === $inner ? 1 : 0), "\n";
echo "file_unchanged: ", ($top->getFile() === $inner->getFile() ? 1 : 0), "\n";
