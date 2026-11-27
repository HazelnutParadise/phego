<?php
// try_multiple_catch_order: the first matching catch runs even when a later one matches more exactly; non matching clauses are skipped
class AppError extends Exception {}
class DbError extends AppError {}

function run($kind)
{
    if ($kind === 'db') {
        throw new DbError("db failed");
    }
    if ($kind === 'app') {
        throw new AppError("app failed");
    }
    if ($kind === 'plain') {
        throw new Exception("plain failed");
    }
    if ($kind === 'err') {
        throw new Error("engine-level failed");
    }
    return "none";
}

foreach (['db', 'app', 'plain', 'err', 'ok'] as $kind) {
    try {
        $r = run($kind);
        echo $kind, " result: ", $r, "\n";
    } catch (Exception $e) {
        echo $kind, " caught by Exception clause: ", $e::class, "\n";
    } catch (AppError $e) {
        echo $kind, " caught by AppError clause: ", $e::class, "\n";
    } catch (DbError $e) {
        echo $kind, " caught by DbError clause: ", $e::class, "\n";
    } catch (Throwable $e) {
        echo $kind, " caught by Throwable clause: ", $e::class, "\n";
    }
}

try {
    run('app');
} catch (DbError $e) {
    echo "wrong: DbError clause\n";
} catch (AppError $e) {
    echo "skipped DbError, AppError clause got: ", $e->getMessage(), "\n";
}

try {
    run('err');
} catch (Exception $e) {
    echo "wrong: Exception clause for Error\n";
} catch (Throwable $e) {
    echo "Throwable clause got Error: ", $e->getMessage(), "\n";
}
