import type { Metadata } from "next";
import { Container } from "@/components/ui";
import { NearbyPharmacies } from "./nearby-pharmacies";
import { T } from "@/components/t";

export const metadata: Metadata = { title: "Pharmacies near you" };

export default function PharmaciesPage() {
  return (
    <Container className="max-w-3xl py-8">
      <h1 className="mb-2 text-2xl font-semibold text-slate-900"><T>Pharmacies near you</T></h1>
      <p className="mb-6 text-sm text-slate-600">
        <T>Verified pharmacies, nearest first. To order, send your prescription and let nearby pharmacies quote.</T>
      </p>
      <NearbyPharmacies />
    </Container>
  );
}
