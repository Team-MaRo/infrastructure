<?php
// Run by hand, not deployed (README "Google Analytics history"): prepares an EXISTING Matomo
// site for a GoogleAnalyticsImporter GA4 import, as the plugin's WordPress path does - the site's
// creation date back to the first imported day, and the importer's status for the site, so
// `import-ga4-reports --idsite=<n>` resumes into it instead of creating a new site.
// Usage, copied into the web pod: php ga-prepare-site.php <idSite> <propertyId> <startDate> <endDate> [apply]
// Without "apply" it only prints what it would do. It refuses a site with raw visits on or before
// the end date (those days would be both imported and tracked) or with an importer status already.
define('PIWIK_INCLUDE_PATH', '/var/www/html');
define('PIWIK_USER_PATH', '/var/www/html');
define('PIWIK_ENABLE_DISPATCH', false);
define('PIWIK_ENABLE_ERROR_HANDLER', false);
define('PIWIK_ENABLE_SESSION_START', false);
require_once PIWIK_INCLUDE_PATH . '/index.php';
$environment = new \Piwik\Application\Environment(null);
$environment->init();
\Piwik\FrontController::getInstance()->init();

[, $idSite, $propertyId, $start, $end] = $argv + [null, null, null, null, null];
$apply = ($argv[5] ?? '') === 'apply';
if (!ctype_digit((string) $idSite) || !ctype_digit((string) $propertyId) || !$start || !$end) {
    fwrite(STDERR, "usage: php ga-prepare-site.php <idSite> <propertyId> <startDate> <endDate> [apply]\n");
    exit(1);
}

\Piwik\Access::doAsSuperUser(function () use ($idSite, $propertyId, $start, $end, $apply) {
    $status = \Piwik\Container\StaticContainer::get(\Piwik\Plugins\GoogleAnalyticsImporter\ImportStatus::class);
    $site = \Piwik\Plugins\SitesManager\API::getInstance()->getSiteFromId($idSite);
    $firstRaw = \Piwik\Db::fetchOne('SELECT MIN(visit_first_action_time) FROM ' . \Piwik\Common::prefixTable('log_visit') . ' WHERE idsite = ?', [$idSite]);
    $current = null;
    try {
        $current = $status->getImportStatus($idSite);
    } catch (\Exception $e) {
        // no import status yet - expected
    }

    printf("site %d \"%s\": created %s, first raw visit %s, importer status %s\n",
        $idSite, $site['name'], $site['ts_created'], $firstRaw ?: '-', $current ? $current['status'] : 'none');
    printf("would: creation date -> %s; import status for properties/%s (accounts/69484706), range %s .. %s\n",
        $start, $propertyId, $start, $end);

    if ($firstRaw && \Piwik\Date::factory($firstRaw)->toString() <= $end) {
        throw new \Exception("refusing: the site has raw visits on or before the end date $end");
    }
    if ($current) {
        throw new \Exception('refusing: the site already has an importer status');
    }
    if (!$apply) {
        echo "dry run - nothing changed\n";
        return;
    }

    \Piwik\API\Request::processRequest('SitesManager.updateSite', ['idSite' => $idSite, 'startDate' => $start]);
    $status->startingImport("properties/$propertyId", 'accounts/69484706', '', $idSite, [], 'ga4', []);
    $status->setImportDateRange($idSite, \Piwik\Date::factory($start), \Piwik\Date::factory($end));
    $site = \Piwik\Plugins\SitesManager\API::getInstance()->getSiteFromId($idSite);
    $now = $status->getImportStatus($idSite);
    printf("done: created %s, status %s, range %s .. %s\n", $site['ts_created'], $now['status'], $now['import_range_start'], $now['import_range_end']);
});
